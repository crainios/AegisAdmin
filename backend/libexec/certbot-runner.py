#!/usr/bin/env python3

"""
Exécuteur asynchrone des actions Certbot d’AegisAdmin.

Ce programme est lancé par une unité systemd temporaire créée par le
domaine backend certbot. Il ne constitue pas une commande publique.
"""

from __future__ import annotations

import fcntl
import json
import os
import pathlib
import re
import selectors
import signal
import stat
import subprocess
import sys
import time
from typing import BinaryIO


def first_executable(candidates: tuple[str, ...]) -> str:
    for candidate in candidates:
        path = pathlib.Path(candidate)
        if path.is_file() and os.access(path, os.X_OK):
            return candidate
    return candidates[0]


def apache_sites_directory() -> pathlib.Path:
    profile = pathlib.Path("/etc/aegisadmin-system/apache")
    try:
        for raw_line in profile.read_text(encoding="utf-8").splitlines():
            line = raw_line.strip()
            if line.startswith("sites_enabled="):
                value = pathlib.Path(line.split("=", 1)[1].strip())
                if value.is_absolute():
                    return value
    except (OSError, UnicodeError):
        pass
    return pathlib.Path("/etc/apache2/sites-enabled")


CERTBOT_COMMAND = first_executable(
    ("/usr/bin/certbot", "/usr/local/bin/certbot")
)
LETSENCRYPT_LIVE_DIRECTORY = pathlib.Path(
    "/etc/letsencrypt/live"
)
LETSENCRYPT_RENEWAL_DIRECTORY = pathlib.Path(
    "/etc/letsencrypt/renewal"
)
APACHE_SITES_ENABLED_DIRECTORY = apache_sites_directory()
RESULT_DIRECTORY = pathlib.Path(
    "/run/aegisadmin-system/certbot"
)
LOCK_FILE = pathlib.Path(
    "/run/aegisadmin-system/certbot.lock"
)
EXECUTION_ID = re.compile(r"^[a-f0-9]{32}$")
DOMAIN_NAME = re.compile(
    r"^(?=.{1,253}$)"
    r"(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+"
    r"[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$"
)
EMAIL_ADDRESS = re.compile(
    r"^[A-Za-z0-9.!#$%&'*+/=?^_{}|~-]+@"
    r"[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?"
    r"(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+$"
)
CERTIFICATE_NAME = re.compile(
    r"^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$"
)
ALLOWED_ACTIONS = {
    "renew-test",
    "renew",
    "issue",
    "delete",
    "reinstall",
    "renew-replace",
}
MAX_DOMAINS = 20
MAX_DURATION_SECONDS = 900
TERMINATION_GRACE_SECONDS = 5
MAX_OUTPUT_BYTES = 128 * 1024
MAX_RESULT_BYTES = 512 * 1024
RESULT_RETENTION_SECONDS = 24 * 60 * 60


class RunnerError(RuntimeError):
    """Erreur interne contrôlée de l’exécuteur."""


def secure_directory() -> None:
    try:
        directory_status = RESULT_DIRECTORY.stat(
            follow_symlinks=False
        )
    except OSError as exception:
        raise RunnerError(
            "Le répertoire des résultats Certbot est indisponible."
        ) from exception

    if (
        not stat.S_ISDIR(directory_status.st_mode)
        or directory_status.st_uid != 0
        or directory_status.st_gid != 0
        or directory_status.st_mode & 0o077 != 0
    ):
        raise RunnerError(
            "Le répertoire des résultats Certbot n’est pas sécurisé."
        )


def result_path(execution_id: str) -> pathlib.Path:
    return RESULT_DIRECTORY / f"{execution_id}.json"


def validate_result_file(path: pathlib.Path) -> None:
    try:
        file_status = path.stat(
            follow_symlinks=False
        )
    except OSError as exception:
        raise RunnerError(
            "Le fichier de résultat Certbot est introuvable."
        ) from exception

    if (
        not stat.S_ISREG(file_status.st_mode)
        or file_status.st_uid != 0
        or file_status.st_gid != 0
        or file_status.st_mode & 0o077 != 0
        or file_status.st_size > MAX_RESULT_BYTES
    ):
        raise RunnerError(
            "Le fichier de résultat Certbot n’est pas sécurisé."
        )


def read_placeholder(
    path: pathlib.Path,
    execution_id: str,
    action: str,
) -> dict[str, object]:
    validate_result_file(path)

    try:
        payload = json.loads(
            path.read_text(
                encoding="utf-8",
                errors="strict",
            )
        )
    except (
        OSError,
        UnicodeError,
        ValueError,
        json.JSONDecodeError,
    ) as exception:
        raise RunnerError(
            "Le résultat Certbot initial est invalide."
        ) from exception

    if (
        not isinstance(payload, dict)
        or payload.get("execution_id") != execution_id
        or payload.get("action") != action
        or payload.get("status") != "running"
    ):
        raise RunnerError(
            "Le résultat Certbot initial est incohérent."
        )

    if action not in {
        "issue",
        "delete",
        "reinstall",
        "renew-replace",
    }:
        return {}

    parameters = payload.get("parameters")

    if not isinstance(parameters, dict):
        raise RunnerError(
            "Les paramètres de l’action Certbot sont absents."
        )

    if action in {
        "delete",
        "reinstall",
        "renew-replace",
    }:
        certificate_name = parameters.get("certificate_name")

        if (
            not isinstance(certificate_name, str)
            or CERTIFICATE_NAME.fullmatch(certificate_name) is None
        ):
            raise RunnerError(
                "Le nom du certificat Certbot est invalide."
            )

        return {
            "certificate_name": certificate_name,
        }

    domains = parameters.get("domains")
    email = parameters.get("email")
    redirect = parameters.get("redirect")

    if (
        not isinstance(domains, list)
        or not 1 <= len(domains) <= MAX_DOMAINS
        or not isinstance(email, str)
        or not 3 <= len(email) <= 254
        or EMAIL_ADDRESS.fullmatch(email) is None
        or not isinstance(redirect, bool)
    ):
        raise RunnerError(
            "Les paramètres d’émission Certbot sont invalides."
        )

    normalized_domains: list[str] = []

    for domain in domains:
        if not isinstance(domain, str):
            raise RunnerError(
                "Un domaine Certbot est invalide."
            )

        normalized_domain = domain.strip().lower()

        if (
            normalized_domain != domain
            or DOMAIN_NAME.fullmatch(normalized_domain) is None
            or normalized_domain.endswith(".onion")
            or normalized_domain in normalized_domains
        ):
            raise RunnerError(
                "Un domaine Certbot est invalide."
            )

        normalized_domains.append(normalized_domain)

    return {
        "domains": normalized_domains,
        "email": email,
        "redirect": redirect,
    }


def open_lock() -> int:
    flags = os.O_RDWR | os.O_CREAT

    if hasattr(os, "O_NOFOLLOW"):
        flags |= os.O_NOFOLLOW

    try:
        descriptor = os.open(
            LOCK_FILE,
            flags,
            0o600,
        )
    except OSError as exception:
        raise RunnerError(
            "Le verrou Certbot n’a pas pu être ouvert."
        ) from exception

    try:
        lock_status = os.fstat(descriptor)

        if (
            not stat.S_ISREG(lock_status.st_mode)
            or lock_status.st_uid != 0
            or lock_status.st_gid != 0
            or lock_status.st_mode & 0o077 != 0
        ):
            raise RunnerError(
                "Le verrou Certbot n’est pas sécurisé."
            )

        fcntl.flock(
            descriptor,
            fcntl.LOCK_EX | fcntl.LOCK_NB,
        )
    except BlockingIOError as exception:
        os.close(descriptor)

        raise RunnerError(
            "Une autre action Certbot est déjà en cours."
        ) from exception
    except Exception:
        os.close(descriptor)
        raise

    return descriptor


def ensure_certificate_can_be_deleted(
    certificate_name: str,
) -> None:
    renewal_path = (
        LETSENCRYPT_RENEWAL_DIRECTORY
        / f"{certificate_name}.conf"
    )

    try:
        renewal_status = renewal_path.stat(
            follow_symlinks=False
        )
    except OSError as exception:
        raise RunnerError(
            "Le certificat Certbot à supprimer est introuvable."
        ) from exception

    if (
        not stat.S_ISREG(renewal_status.st_mode)
        or renewal_status.st_uid != 0
        or renewal_status.st_gid != 0
    ):
        raise RunnerError(
            "La configuration de renouvellement Certbot n’est pas sécurisée."
        )

    certificate_directory = str(
        LETSENCRYPT_LIVE_DIRECTORY / certificate_name
    ) + "/"

    try:
        enabled_sites = sorted(
            APACHE_SITES_ENABLED_DIRECTORY.iterdir(),
            key=lambda path: path.name,
        )
    except OSError as exception:
        raise RunnerError(
            "Les sites Apache actifs n’ont pas pu être vérifiés."
        ) from exception

    for enabled_site in enabled_sites:
        try:
            if not enabled_site.is_file():
                continue

            content = enabled_site.read_text(
                encoding="utf-8",
                errors="strict",
            )
        except (OSError, UnicodeError) as exception:
            raise RunnerError(
                "Un site Apache actif n’a pas pu être vérifié."
            ) from exception

        if certificate_directory in content:
            raise RunnerError(
                "Le certificat est encore utilisé par le site Apache actif "
                f"{enabled_site.name}."
            )


def action_command(
    action: str,
    parameters: dict[str, object],
) -> list[str]:
    if action == "issue":
        domains = parameters.get("domains")
        email = parameters.get("email")
        redirect = parameters.get("redirect")

        if (
            not isinstance(domains, list)
            or not all(
                isinstance(domain, str)
                for domain in domains
            )
            or not isinstance(email, str)
            or not isinstance(redirect, bool)
        ):
            raise RunnerError(
                "Les paramètres d’émission Certbot sont invalides."
            )

        command = [
            CERTBOT_COMMAND,
            "--apache",
            "--non-interactive",
            "--agree-tos",
            "--email",
            email,
            "--redirect" if redirect else "--no-redirect",
        ]

        for domain in domains:
            command.extend([
                "--domain",
                domain,
            ])

        return command

    if action in {
        "delete",
        "reinstall",
        "renew-replace",
    }:
        certificate_name = parameters.get("certificate_name")

        if not isinstance(certificate_name, str):
            raise RunnerError(
                "Le nom du certificat Certbot est invalide."
            )

        if action == "delete":
            ensure_certificate_can_be_deleted(
                certificate_name
            )

            return [
                CERTBOT_COMMAND,
                "delete",
                "--cert-name",
                certificate_name,
                "--non-interactive",
            ]

        if action == "reinstall":
            return [
                CERTBOT_COMMAND,
                "install",
                "--cert-name",
                certificate_name,
                "--apache",
                "--non-interactive",
            ]

        return [
            CERTBOT_COMMAND,
            "renew",
            "--cert-name",
            certificate_name,
            "--force-renewal",
            "--no-random-sleep-on-renew",
        ]

    command = [
        CERTBOT_COMMAND,
        "renew",
    ]

    if action == "renew-test":
        command.append("--dry-run")

    command.append("--no-random-sleep-on-renew")

    return command


def append_output(
    destination: bytearray,
    chunk: bytes,
    remaining: int,
) -> tuple[int, bool]:
    if remaining <= 0:
        return 0, bool(chunk)

    retained = chunk[:remaining]
    destination.extend(retained)

    return (
        len(retained),
        len(retained) < len(chunk),
    )


def terminate_process(process: subprocess.Popen[bytes]) -> None:
    if process.poll() is not None:
        return

    try:
        os.killpg(
            process.pid,
            signal.SIGTERM,
        )
    except ProcessLookupError:
        return

    try:
        process.wait(
            timeout=TERMINATION_GRACE_SECONDS
        )
        return
    except subprocess.TimeoutExpired:
        pass

    try:
        os.killpg(
            process.pid,
            signal.SIGKILL,
        )
    except ProcessLookupError:
        return

    process.wait()


def execute(
    action: str,
    parameters: dict[str, object],
) -> tuple[int | None, bool, bool, int, str, str]:
    environment = {
        "HOME": "/root",
        "LANG": "C",
        "LC_ALL": "C",
        "PATH": "/usr/sbin:/usr/bin:/sbin:/bin",
    }
    started_at = time.monotonic()
    stdout = bytearray()
    stderr = bytearray()
    retained_bytes = 0
    truncated = False
    timed_out = False

    try:
        process = subprocess.Popen(
            action_command(
                action,
                parameters,
            ),
            stdin=subprocess.DEVNULL,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env=environment,
            start_new_session=True,
        )
    except OSError as exception:
        raise RunnerError(
            "Certbot n’a pas pu être exécuté."
        ) from exception

    if process.stdout is None or process.stderr is None:
        terminate_process(process)

        raise RunnerError(
            "Les sorties Certbot sont indisponibles."
        )

    selector = selectors.DefaultSelector()
    streams: dict[int, tuple[BinaryIO, bytearray]] = {}

    try:
        for stream, destination in (
            (process.stdout, stdout),
            (process.stderr, stderr),
        ):
            os.set_blocking(
                stream.fileno(),
                False,
            )
            selector.register(
                stream,
                selectors.EVENT_READ,
            )
            streams[stream.fileno()] = (
                stream,
                destination,
            )

        while streams:
            elapsed = time.monotonic() - started_at
            remaining_time = (
                MAX_DURATION_SECONDS - elapsed
            )

            if remaining_time <= 0:
                timed_out = True
                terminate_process(process)

            events = selector.select(
                timeout=(
                    0.1
                    if timed_out
                    else min(0.5, remaining_time)
                )
            )

            for key, _ in events:
                stream = key.fileobj
                file_descriptor = stream.fileno()

                try:
                    chunk = os.read(
                        file_descriptor,
                        16 * 1024,
                    )
                except BlockingIOError:
                    continue

                if not chunk:
                    selector.unregister(stream)
                    streams.pop(
                        file_descriptor,
                        None,
                    )
                    stream.close()
                    continue

                destination = streams[
                    file_descriptor
                ][1]
                retained, was_truncated = append_output(
                    destination,
                    chunk,
                    MAX_OUTPUT_BYTES - retained_bytes,
                )
                retained_bytes += retained
                truncated = (
                    truncated or was_truncated
                )

            if timed_out and process.poll() is not None:
                for (
                    file_descriptor,
                    (stream, _),
                ) in list(streams.items()):
                    try:
                        while True:
                            chunk = os.read(
                                file_descriptor,
                                16 * 1024,
                            )

                            if not chunk:
                                break

                            destination = streams[
                                file_descriptor
                            ][1]
                            (
                                retained,
                                was_truncated,
                            ) = append_output(
                                destination,
                                chunk,
                                (
                                    MAX_OUTPUT_BYTES
                                    - retained_bytes
                                ),
                            )
                            retained_bytes += retained
                            truncated = (
                                truncated
                                or was_truncated
                            )
                    except BlockingIOError:
                        pass

                    selector.unregister(stream)
                    streams.pop(
                        file_descriptor,
                        None,
                    )
                    stream.close()

        if process.poll() is None:
            process.wait()
    finally:
        selector.close()
        terminate_process(process)

        for stream in (
            process.stdout,
            process.stderr,
        ):
            if not stream.closed:
                stream.close()

    duration_ms = int(
        (time.monotonic() - started_at) * 1000
    )

    return (
        None if timed_out else process.returncode,
        timed_out,
        truncated,
        duration_ms,
        stdout.decode(
            "utf-8",
            errors="replace",
        ),
        stderr.decode(
            "utf-8",
            errors="replace",
        ),
    )


def write_result(
    path: pathlib.Path,
    result: dict[str, object],
) -> None:
    encoded = (
        json.dumps(
            result,
            ensure_ascii=False,
            separators=(",", ":"),
        )
        + "\n"
    ).encode("utf-8")

    if len(encoded) > MAX_RESULT_BYTES:
        raise RunnerError(
            "Le résultat Certbot dépasse la taille autorisée."
        )

    temporary_path = path.with_name(
        f".{path.name}.{os.getpid()}.tmp"
    )
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL

    if hasattr(os, "O_NOFOLLOW"):
        flags |= os.O_NOFOLLOW

    try:
        descriptor = os.open(
            temporary_path,
            flags,
            0o600,
        )

        with os.fdopen(
            descriptor,
            "wb",
        ) as stream:
            stream.write(encoded)
            stream.flush()
            os.fsync(stream.fileno())

        os.replace(
            temporary_path,
            path,
        )
        validate_result_file(path)
    except OSError as exception:
        try:
            temporary_path.unlink(
                missing_ok=True
            )
        except OSError:
            pass

        raise RunnerError(
            "Le résultat Certbot n’a pas pu être enregistré."
        ) from exception


def cleanup_results(
    current_path: pathlib.Path,
) -> None:
    threshold = time.time() - RESULT_RETENTION_SECONDS

    try:
        paths = tuple(
            RESULT_DIRECTORY.iterdir()
        )
    except OSError:
        return

    for path in paths:
        if path == current_path:
            continue

        if not re.fullmatch(
            r"[a-f0-9]{32}\.json",
            path.name,
        ):
            continue

        try:
            file_status = path.stat(
                follow_symlinks=False
            )

            if (
                stat.S_ISREG(file_status.st_mode)
                and file_status.st_uid == 0
                and file_status.st_gid == 0
                and file_status.st_mtime < threshold
            ):
                path.unlink()
        except OSError:
            continue


def failure_result(
    execution_id: str,
    action: str,
    message: str,
) -> dict[str, object]:
    return {
        "execution_id": execution_id,
        "action": action,
        "status": "failed",
        "exit_code": None,
        "timed_out": False,
        "truncated": False,
        "duration_ms": 0,
        "stdout": "",
        "stderr": message,
    }


def main() -> int:
    if len(sys.argv) != 3:
        print(
            "Nombre d’arguments invalide.",
            file=sys.stderr,
        )
        return 2

    action = sys.argv[1]
    execution_id = sys.argv[2]

    if action not in ALLOWED_ACTIONS:
        print(
            "Action Certbot invalide.",
            file=sys.stderr,
        )
        return 2

    if EXECUTION_ID.fullmatch(execution_id) is None:
        print(
            "Identifiant d’exécution Certbot invalide.",
            file=sys.stderr,
        )
        return 2

    path = result_path(execution_id)
    lock_descriptor: int | None = None

    try:
        secure_directory()
        parameters = read_placeholder(
            path,
            execution_id,
            action,
        )
        cleanup_results(path)
        lock_descriptor = open_lock()
        (
            exit_code,
            timed_out,
            truncated,
            duration_ms,
            stdout,
            stderr,
        ) = execute(
            action,
            parameters,
        )
        result: dict[str, object] = {
            "execution_id": execution_id,
            "action": action,
            "status": "finished",
            "exit_code": exit_code,
            "timed_out": timed_out,
            "truncated": truncated,
            "duration_ms": duration_ms,
            "stdout": stdout,
            "stderr": stderr,
        }
    except RunnerError as exception:
        result = failure_result(
            execution_id,
            action,
            str(exception),
        )

    try:
        write_result(
            path,
            result,
        )
    except RunnerError as exception:
        print(
            str(exception),
            file=sys.stderr,
        )

        if lock_descriptor is not None:
            os.close(lock_descriptor)

        return 10

    if lock_descriptor is not None:
        os.close(lock_descriptor)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
