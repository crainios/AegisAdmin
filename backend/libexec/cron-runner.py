#!/usr/bin/python3

"""Exécute temporairement une tâche Cron avec des limites strictes."""

from __future__ import annotations

import json
import os
import pwd
import re
import selectors
import signal
import subprocess
import sys
import time
from pathlib import Path
from typing import BinaryIO

ALLOWED_USERS_FILE = Path("/etc/aegisadmin-system/cron-users")
DENIED_USERS_FILE = Path("/etc/aegisadmin-system/cron-users-deny")
LOGIN_DEFS_FILE = Path("/etc/login.defs")
EXECUTION_ID_PATTERN = re.compile(r"^[a-f0-9]{32}$")
RESULT_DIRECTORY = Path("/run/aegisadmin-system/cron")
MAX_COMMAND_LENGTH = 8192
MAX_DURATION_SECONDS = 60.0
MAX_OUTPUT_BYTES = 64 * 1024
READ_CHUNK_SIZE = 8192
TERMINATION_GRACE_SECONDS = 1.0


def fail(message: str) -> "NoReturn":
    print(message, file=sys.stderr)
    raise SystemExit(2)


def allowed_user(name: str) -> bool:
    minimum = 1000
    try:
        for raw in LOGIN_DEFS_FILE.read_text(encoding="utf-8").splitlines():
            fields = raw.split("#", 1)[0].split()
            if len(fields) == 2 and fields[0] == "UID_MIN":
                minimum = int(fields[1])
                break
    except (OSError, UnicodeError, ValueError):
        pass

    explicit = set()
    try:
        for raw in ALLOWED_USERS_FILE.read_text(encoding="utf-8").splitlines():
            value = raw.split("#", 1)[0].strip()
            if value:
                explicit.add(value)
    except FileNotFoundError:
        pass

    denied = set()
    try:
        for raw in DENIED_USERS_FILE.read_text(encoding="utf-8").splitlines():
            value = raw.split("#", 1)[0].strip()
            if value:
                denied.add(value)
    except FileNotFoundError:
        pass

    try:
        account = pwd.getpwnam(name)
    except KeyError:
        return False

    shell = os.path.basename(account.pw_shell)
    human = (
        account.pw_uid >= minimum
        and os.path.isabs(account.pw_dir)
        and os.path.isabs(account.pw_shell)
        and shell not in {"false", "nologin"}
    )
    return (
        name != "root"
        and account.pw_uid != 65534
        and name not in denied
        and (human or name in explicit)
    )


def validate_arguments() -> tuple[str, str, str]:
    if len(sys.argv) != 4:
        fail("Le nombre d’arguments fourni au lanceur Cron est invalide.")

    execution_id = sys.argv[1]
    user = sys.argv[2]
    command = sys.argv[3]

    if EXECUTION_ID_PATTERN.fullmatch(execution_id) is None:
        fail("L’identifiant d’exécution Cron est invalide.")

    if not allowed_user(user):
        fail("L’utilisateur Cron demandé n’est pas autorisé.")

    if (
        not command
        or len(command) > MAX_COMMAND_LENGTH
        or "\x00" in command
        or "\r" in command
        or "\n" in command
    ):
        fail("La commande Cron demandée est invalide.")

    return execution_id, user, command


def validate_result_directory() -> None:
    try:
        status = RESULT_DIRECTORY.stat(follow_symlinks=False)
    except FileNotFoundError:
        fail("Le répertoire de résultats Cron est absent.")

    if (
        not RESULT_DIRECTORY.is_dir()
        or status.st_uid != 0
        or status.st_mode & 0o022 != 0
    ):
        fail("Le répertoire de résultats Cron n’est pas sécurisé.")


def result_path(execution_id: str) -> Path:
    return RESULT_DIRECTORY / f"{execution_id}.json"


def write_result(execution_id: str, result: dict[str, object]) -> None:
    destination = result_path(execution_id)
    temporary = RESULT_DIRECTORY / (
        f".{execution_id}.{os.getpid()}.tmp"
    )
    payload = json.dumps(
        result,
        ensure_ascii=False,
        separators=(",", ":"),
    ).encode("utf-8")
    descriptor = os.open(
        temporary,
        os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW,
        0o600,
    )

    try:
        with os.fdopen(descriptor, "wb") as stream:
            stream.write(payload)
            stream.write(b"\n")
            stream.flush()
            os.fsync(stream.fileno())

        os.replace(temporary, destination)
        os.chmod(destination, 0o600, follow_symlinks=False)
    finally:
        try:
            temporary.unlink()
        except FileNotFoundError:
            pass


def child_identity(user: str) -> tuple[pwd.struct_passwd, str]:
    try:
        account = pwd.getpwnam(user)
    except KeyError:
        fail("Le compte Unix demandé n’existe pas.")

    home = account.pw_dir

    if not home or not os.path.isdir(home):
        home = "/"

    return account, home


def drop_privileges(account: pwd.struct_passwd) -> None:
    os.setgroups([])
    os.setgid(account.pw_gid)
    os.setuid(account.pw_uid)


def terminate_process_group(process: subprocess.Popen[bytes]) -> None:
    if process.poll() is not None:
        return

    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        return

    try:
        process.wait(timeout=TERMINATION_GRACE_SECONDS)
        return
    except subprocess.TimeoutExpired:
        pass

    try:
        os.killpg(process.pid, signal.SIGKILL)
    except ProcessLookupError:
        return

    try:
        process.wait(timeout=TERMINATION_GRACE_SECONDS)
    except subprocess.TimeoutExpired:
        pass


def capture_output(
    process: subprocess.Popen[bytes],
) -> tuple[bytes, bytes, bool, bool]:
    selector = selectors.DefaultSelector()
    streams: dict[BinaryIO, str] = {}
    captured = {
        "stdout": bytearray(),
        "stderr": bytearray(),
    }
    stored_bytes = 0
    truncated = False
    timed_out = False
    deadline = time.monotonic() + MAX_DURATION_SECONDS

    if process.stdout is not None:
        selector.register(process.stdout, selectors.EVENT_READ)
        streams[process.stdout] = "stdout"

    if process.stderr is not None:
        selector.register(process.stderr, selectors.EVENT_READ)
        streams[process.stderr] = "stderr"

    try:
        while selector.get_map():
            remaining_time = deadline - time.monotonic()

            if remaining_time <= 0:
                timed_out = True
                terminate_process_group(process)
                break

            events = selector.select(
                timeout=min(0.2, remaining_time)
            )

            if not events and process.poll() is not None:
                events = selector.select(timeout=0)

                if not events:
                    break

            for key, _ in events:
                stream = key.fileobj
                chunk = stream.read(READ_CHUNK_SIZE)

                if not chunk:
                    selector.unregister(stream)
                    continue

                available = MAX_OUTPUT_BYTES - stored_bytes

                if available > 0:
                    retained = chunk[:available]
                    captured[streams[stream]].extend(retained)
                    stored_bytes += len(retained)

                if len(chunk) > available:
                    truncated = True
    finally:
        selector.close()

        if timed_out:
            terminate_process_group(process)

        for stream in streams:
            stream.close()

    if process.poll() is None:
        try:
            process.wait(timeout=TERMINATION_GRACE_SECONDS)
        except subprocess.TimeoutExpired:
            timed_out = True
            terminate_process_group(process)

    return (
        bytes(captured["stdout"]),
        bytes(captured["stderr"]),
        timed_out,
        truncated,
    )


def execute(user: str, command: str) -> dict[str, object]:
    account, home = child_identity(user)
    environment = {
        "HOME": home,
        "LANG": "C.UTF-8",
        "LOGNAME": user,
        "PATH": "/usr/local/bin:/usr/bin:/bin",
        "SHELL": "/bin/sh",
        "USER": user,
    }
    started_at = time.monotonic()
    process = subprocess.Popen(
        ["/bin/sh", "-c", command],
        cwd=home,
        env=environment,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        preexec_fn=lambda: drop_privileges(account),
        start_new_session=True,
    )
    stdout, stderr, timed_out, truncated = capture_output(process)
    duration_ms = max(
        0,
        round((time.monotonic() - started_at) * 1000),
    )

    return {
        "status": "finished",
        "exit_code": None if timed_out else process.returncode,
        "timed_out": timed_out,
        "truncated": truncated,
        "duration_ms": duration_ms,
        "stdout": stdout.decode("utf-8", errors="replace"),
        "stderr": stderr.decode("utf-8", errors="replace"),
    }


def main() -> int:
    execution_id, user, command = validate_arguments()
    validate_result_directory()

    try:
        result = execute(user, command)
    except Exception:
        result = {
            "status": "failed",
            "exit_code": None,
            "timed_out": False,
            "truncated": False,
            "duration_ms": 0,
            "stdout": "",
            "stderr": "L’exécution de la tâche Cron a échoué.",
        }

    result["execution_id"] = execution_id
    write_result(execution_id, result)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
