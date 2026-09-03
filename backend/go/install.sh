#!/usr/bin/env bash

set -euo pipefail

IFS=$'\n\t'
umask 027

export PATH="/usr/local/go/bin:/opt/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
export LANG="C"
export LC_ALL="C"

readonly SCRIPT_DIR="$(
    cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
    pwd -P
)"
readonly APP_ROOT="$(cd -- "${SCRIPT_DIR}/../.." && pwd -P)"
readonly DAEMON_TARGET="/usr/libexec/aegisadmin/aegisadmin-daemon"
readonly CLI_TARGET="/usr/bin/aegisadmin-system-go"
readonly ADMIN_TARGET="/usr/libexec/aegisadmin/aegisadmin-admin"
readonly WEB_TARGET="/usr/libexec/aegisadmin/aegisadmin-web"
readonly UPDATER_TARGET="/usr/libexec/aegisadmin/aegisadmin-updater"
readonly BACKUP_TARGET="/usr/libexec/aegisadmin/aegisadmin-backup"
readonly UNIT_TARGET="/usr/lib/systemd/system/aegisadmin-system.service"
readonly UPDATER_UNIT_TARGET="/usr/lib/systemd/system/aegisadmin-updater@.service"
readonly SUDOERS_TARGET="/etc/sudoers.d/aegisadmin"
readonly WEB_GROUP="aegisadmin-web"
readonly SYSTEM_CONFIG_DIRECTORY="/etc/aegisadmin-system"
readonly TLS_DIRECTORY="${SYSTEM_CONFIG_DIRECTORY}/tls"
readonly DOCUMENTATION_DIRECTORY="/usr/local/share/doc/aegisadmin-system-go"
readonly DOCUMENTATION_TARGET="${DOCUMENTATION_DIRECTORY}/DEPLOYMENT.md"
readonly CRON_RUNNER_SOURCE="${SCRIPT_DIR}/../libexec/cron-runner.py"
readonly CRON_RUNNER_TARGET="/usr/libexec/aegisadmin/cron-runner.py"
readonly CERTBOT_RUNNER_SOURCE="${SCRIPT_DIR}/../libexec/certbot-runner.py"
readonly CERTBOT_RUNNER_TARGET="/usr/libexec/aegisadmin/certbot-runner.py"
readonly CRON_USERS_SOURCE="${SCRIPT_DIR}/../config/cron-users"
readonly CRON_USERS_TARGET="/etc/aegisadmin-system/cron-users"
readonly CRON_USERS_DENY_SOURCE="${SCRIPT_DIR}/../config/cron-users-deny"
readonly CRON_USERS_DENY_TARGET="/etc/aegisadmin-system/cron-users-deny"
readonly SERVICES_SOURCE="${SCRIPT_DIR}/../config/services"
readonly SERVICES_TARGET="/etc/aegisadmin-system/services"
readonly APACHE_PROFILE_SOURCE="${SCRIPT_DIR}/../config/apache"
readonly APACHE_PROFILE_TARGET="/etc/aegisadmin-system/apache"
readonly LOGS_POLICY_SOURCE="${SCRIPT_DIR}/../config/logs"
readonly LOGS_POLICY_TARGET="/etc/aegisadmin-system/logs"
readonly FAIL2BAN_PROFILE_SOURCE="${SCRIPT_DIR}/../config/fail2ban"
readonly FAIL2BAN_PROFILE_TARGET="/etc/aegisadmin-system/fail2ban"
readonly TOR_PROFILE_SOURCE="${SCRIPT_DIR}/../config/tor"
readonly TOR_PROFILE_TARGET="/etc/aegisadmin-system/tor"
readonly CRON_PROFILE_SOURCE="${SCRIPT_DIR}/../config/cron"
readonly CRON_PROFILE_TARGET="/etc/aegisadmin-system/cron"
readonly STORAGE_PROFILE_SOURCE="${SCRIPT_DIR}/../config/storage"
readonly STORAGE_PROFILE_TARGET="/etc/aegisadmin-system/storage"
readonly COMPOSER_PROFILE_SOURCE="${SCRIPT_DIR}/../config/composer"
readonly COMPOSER_PROFILE_TARGET="/etc/aegisadmin-system/composer"
readonly APACHE_BACKUP_DIRECTORY="/var/backups/aegisadmin-system/apache"
readonly CRON_BACKUP_DIRECTORY="/var/backups/aegisadmin-system/cron"
readonly APPLICATION_DATABASE_DIRECTORY="/var/lib/aegisadmin/database"
readonly APPLICATION_STATE_DIRECTORY="/var/lib/aegisadmin"
readonly APPLICATION_UPDATE_DIRECTORY="${APPLICATION_STATE_DIRECTORY}/updates"
readonly APPLICATION_DATABASE_FILE="${APPLICATION_DATABASE_DIRECTORY}/aegisadmin.sqlite"
readonly APPLICATION_DATABASE_BACKUP_DIRECTORY="${APPLICATION_DATABASE_DIRECTORY}/backups"
readonly APPLICATION_SESSION_DIRECTORY="${APPLICATION_STATE_DIRECTORY}/sessions"
readonly APPLICATION_SECRET_DIRECTORY="${APPLICATION_STATE_DIRECTORY}/secrets"
readonly MIGRATIONS_SOURCE="${APP_ROOT}/database/migrations"
readonly MIGRATIONS_TARGET="/usr/share/aegisadmin/migrations"
readonly LEGACY_BACKEND_TARGET="/usr/local/sbin/aegisadmin-system"
readonly LEGACY_COMMANDS_TARGET="/usr/local/lib/aegisadmin-system/commands"
readonly LEGACY_DAEMON_TARGET="/usr/local/libexec/aegisadmin-system/aegisadmin-daemon"
readonly LEGACY_CLI_TARGET="/usr/local/sbin/aegisadmin-system-go"
readonly LEGACY_ADMIN_TARGET="/usr/local/sbin/aegisadmin-admin"
readonly LEGACY_UNIT_TARGET="/etc/systemd/system/aegisadmin-daemon.service"
readonly LEGACY_SYSTEM_OVERRIDE_TARGET="/etc/systemd/system/aegisadmin-system.service"
readonly LEGACY_SUDOERS_TARGET="/etc/sudoers.d/aegisadmin-system-go"

GO_COMMAND=""
GOFMT_COMMAND=""
BASH_EXECUTABLE=""
INSTALL_COMMAND=""
PYTHON_COMMAND=""
RM_COMMAND=""
SLEEP_COMMAND=""
SYSTEMCTL_COMMAND=""
VISUDO_COMMAND=""
GETENT_COMMAND=""
GROUPADD_COMMAND=""
USERMOD_COMMAND=""
AWK_COMMAND=""
FIND_COMMAND=""
XARGS_COMMAND=""
MKTEMP_COMMAND=""
CHOWN_COMMAND=""
CHMOD_COMMAND=""
RUNUSER_COMMAND=""
BUILD_DIRECTORY=""
INSTALL_WEB_USER=""
INSTALL_GO="false"

fail()
{
    printf 'ERREUR : %s\n' "${1}" >&2
    exit "${2:-1}"
}

usage()
{
    printf '%s\n' \
        "Usage :" \
        "  ${0} check" \
        "  sudo ${0} verify" \
        "  sudo ${0} install --web-user UTILISATEUR [OPTIONS]" \
        "  sudo ${0} install  # mise à niveau d’une installation existante" \
        "" \
        "Options d’installation :" \
        "  --install-go            Installe Go si absent, sans demander confirmation." \
        "" \
        "L’interface et le backend sont intégralement fournis par Go."
}

parse_install_arguments()
{
    while (( $# > 0 )); do
        case "${1}" in
            --web-user)
                (( $# >= 2 )) || fail "La valeur de --web-user est absente." 2
                INSTALL_WEB_USER="${2}"
                shift 2
                ;;
            --install-go)
                INSTALL_GO="true"
                shift
                ;;
            *)
                usage >&2
                fail "Option d’installation invalide : ${1}" 2
                ;;
        esac
    done
}

require_no_arguments()
{
    if (( $# != 0 )); then
        usage >&2
        fail "Cette commande n’accepte aucun argument supplémentaire." 2
    fi
}

require_command()
{
    local command_path="${1}"
    local label="${2}"

    if [[ ! -x "${command_path}" ]]; then
        fail "La commande ${label} est introuvable : ${command_path}"
    fi
}

resolve_command()
{
    local variable_name="${1}"
    local label="${2}"
    local candidate=""
    local resolved=""
    shift 2

    for candidate in "$@"; do
        resolved="$(command -v "${candidate}" 2>/dev/null || true)"
        if [[ "${resolved}" == /* && -x "${resolved}" && ! -d "${resolved}" ]]; then
            printf -v "${variable_name}" '%s' "${resolved}"
            return 0
        fi
    done

    fail \
        "La commande ${label} est introuvable dans les emplacements système autorisés." \
        3
}

go_is_available()
{
    local go_path=""
    local gofmt_path=""
    go_path="$(command -v go 2>/dev/null || true)"
    gofmt_path="$(command -v gofmt 2>/dev/null || true)"
    [[ "${go_path}" == /* && -x "${go_path}" && "${gofmt_path}" == /* && -x "${gofmt_path}" ]]
}

confirm_go_installation()
{
    local answer=""

    if [[ "${INSTALL_GO}" == "true" ]]; then
        return 0
    fi
    if [[ ! -t 0 ]]; then
        fail \
            "Go est absent. Relancez avec --install-go pour autoriser son installation automatique." \
            3
    fi

    printf '%s' \
        "Go 1.22 ou ultérieur est requis mais absent. L’installer maintenant ? [o/N] " \
        >&2
    read -r answer
    case "${answer}" in
        o|O|oui|OUI|y|Y|yes|YES) return 0 ;;
        *) fail "L’installation de Go a été refusée." 3 ;;
    esac
}

install_go_dependency()
{
    local package_manager=""

    confirm_go_installation
    if package_manager="$(command -v apt-get 2>/dev/null)" && [[ -x "${package_manager}" ]]; then
        "${package_manager}" update
        "${package_manager}" install -y golang-go
    elif package_manager="$(command -v dnf 2>/dev/null)" && [[ -x "${package_manager}" ]]; then
        "${package_manager}" install -y golang
    elif package_manager="$(command -v yum 2>/dev/null)" && [[ -x "${package_manager}" ]]; then
        "${package_manager}" install -y golang
    elif package_manager="$(command -v zypper 2>/dev/null)" && [[ -x "${package_manager}" ]]; then
        "${package_manager}" --non-interactive install go
    elif package_manager="$(command -v pacman 2>/dev/null)" && [[ -x "${package_manager}" ]]; then
        "${package_manager}" --noconfirm -S go
    else
        fail \
            "Go ne peut pas être installé automatiquement : aucun gestionnaire de paquets pris en charge n’a été trouvé." \
            3
    fi

    hash -r
    if ! go_is_available; then
        fail "Le gestionnaire de paquets n’a pas fourni les commandes go et gofmt." 3
    fi
}

ensure_go_for_install()
{
    if go_is_available; then
        return 0
    fi
    install_go_dependency
}

discover_validation_commands()
{
    resolve_command BASH_EXECUTABLE "bash" bash
    resolve_command GO_COMMAND "go" go
    resolve_command GOFMT_COMMAND "gofmt" gofmt
    resolve_command PYTHON_COMMAND "python3" python3
    resolve_command VISUDO_COMMAND "visudo" visudo
    resolve_command FIND_COMMAND "find" find
    resolve_command XARGS_COMMAND "xargs" xargs
}

discover_install_commands()
{
    discover_validation_commands
    resolve_command INSTALL_COMMAND "install" install
    resolve_command RM_COMMAND "rm" rm
    resolve_command SLEEP_COMMAND "sleep" sleep
    resolve_command SYSTEMCTL_COMMAND "systemctl" systemctl
    resolve_command GETENT_COMMAND "getent" getent
    resolve_command GROUPADD_COMMAND "groupadd" groupadd
    resolve_command USERMOD_COMMAND "usermod" usermod
    resolve_command AWK_COMMAND "awk" awk
    resolve_command MKTEMP_COMMAND "mktemp" mktemp
    resolve_command CHOWN_COMMAND "chown" chown
    resolve_command CHMOD_COMMAND "chmod" chmod
    resolve_command RUNUSER_COMMAND "runuser" runuser
}

discover_remove_commands()
{
    resolve_command RM_COMMAND "rm" rm
    resolve_command SYSTEMCTL_COMMAND "systemctl" systemctl
}

discover_verify_commands()
{
    resolve_command SYSTEMCTL_COMMAND "systemctl" systemctl
    resolve_command SLEEP_COMMAND "sleep" sleep
    resolve_command RUNUSER_COMMAND "runuser" runuser
}

validate_go_version()
{
    local version=""
    local major=0
    local minor=0

    version="$("${GO_COMMAND}" env GOVERSION 2>/dev/null || true)"
    if [[ ! "${version}" =~ ^go([0-9]+)\.([0-9]+)([.]|$) ]]; then
        fail "La version de Go n’a pas pu être déterminée : ${version:-inconnue}" 3
    fi

    major="${BASH_REMATCH[1]}"
    minor="${BASH_REMATCH[2]}"
    if (( major < 1 || (major == 1 && minor < 22) )); then
        fail "Go 1.22 ou une version ultérieure est nécessaire : ${version}" 3
    fi
}

require_root()
{
    if (( EUID != 0 )); then
        fail "Cette opération doit être exécutée avec les privilèges root."
    fi
}

validate_web_user()
{
    local web_user="${1}"

    if [[ ! "${web_user}" =~ ^[a-z_][a-z0-9_-]*[$]?$ ]]; then
        fail "Le nom du compte web est invalide : ${web_user}" 2
    fi

    if ! "${GETENT_COMMAND}" passwd "${web_user}" >/dev/null; then
        fail "Le compte web n’existe pas : ${web_user}" 2
    fi
}

legacy_web_user()
{
    if [[ ! -f "${SUDOERS_TARGET}" || -L "${SUDOERS_TARGET}" ]]; then
        return 0
    fi

    "${AWK_COMMAND}" \
        -v command="${CLI_TARGET}" \
        '$1 !~ /^%/ && $2 == "ALL=(root)" && $3 == "NOPASSWD:" && $4 == command && NF == 4 && !seen[$1]++ { print $1; if (++count == 2) exit }' \
        "${SUDOERS_TARGET}"
}

web_group_has_members()
{
    local group_entry=""
    group_entry="$("${GETENT_COMMAND}" group "${WEB_GROUP}" 2>/dev/null || true)"

    [[ -n "${group_entry}" && -n "${group_entry##*:*:*:}" ]]
}

resolve_web_user()
{
    local explicit_user="${1}"
    local migrated_users=""

    if [[ -n "${explicit_user}" ]]; then
        validate_web_user "${explicit_user}"
        printf '%s\n' "${explicit_user}"
        return 0
    fi

    migrated_users="$(legacy_web_user)"
    if [[ -n "${migrated_users}" && "${migrated_users}" != *$'\n'* ]]; then
        validate_web_user "${migrated_users}"
        printf '%s\n' "${migrated_users}"
        return 0
    fi

    if web_group_has_members; then
        return 0
    fi

    fail \
        "Aucun compte web n’est configuré. Relancez avec : sudo ${0} install --web-user UTILISATEUR" \
        2
}

configure_web_identity()
{
    local web_user="${1}"

    if ! "${GETENT_COMMAND}" group "${WEB_GROUP}" >/dev/null; then
        "${GROUPADD_COMMAND}" --system "${WEB_GROUP}"
    fi

    if [[ -n "${web_user}" ]]; then
        "${USERMOD_COMMAND}" -a -G "${WEB_GROUP}" "${web_user}"
        printf 'Compte web autorisé via le groupe %s : %s\n' \
            "${WEB_GROUP}" "${web_user}"
        printf '%s\n' \
            "Redémarrez les processus web/PHP persistants pour appliquer cette nouvelle appartenance."
    fi
}

configure_application_storage()
{
    local sqlite_file=""

	"${INSTALL_COMMAND}" -d -o root -g "${WEB_GROUP}" -m 0750 \
		"${APPLICATION_STATE_DIRECTORY}"
	"${INSTALL_COMMAND}" -d -o aegisadmin -g aegisadmin -m 0700 \
		"${APPLICATION_SESSION_DIRECTORY}" \
		"${APPLICATION_SECRET_DIRECTORY}"
    "${INSTALL_COMMAND}" -d -o root -g "${WEB_GROUP}" -m 2770 \
        "${APPLICATION_DATABASE_DIRECTORY}"
    "${INSTALL_COMMAND}" -d -o root -g "${WEB_GROUP}" -m 2770 \
        "${APPLICATION_DATABASE_BACKUP_DIRECTORY}"

    while IFS= read -r -d '' sqlite_file; do
        "${CHOWN_COMMAND}" root:"${WEB_GROUP}" "${sqlite_file}"
        "${CHMOD_COMMAND}" 0660 "${sqlite_file}"
    done < <(
        "${FIND_COMMAND}" "${APPLICATION_DATABASE_DIRECTORY}" \
            -maxdepth 1 -type f \
            \( -name '*.sqlite' -o -name '*.sqlite-shm' -o -name '*.sqlite-wal' \) \
            -print0
    )

    printf 'Stockage SQLite préparé pour le groupe %s : %s\n' \
        "${WEB_GROUP}" "${APPLICATION_DATABASE_DIRECTORY}"
}

validate_sources()
{
    require_command "${GO_COMMAND}" "go"
    require_command "${GOFMT_COMMAND}" "gofmt"
    require_command "${PYTHON_COMMAND}" "python3"
    validate_go_version

    if [[ ! -f "${CRON_USERS_SOURCE}" || -L "${CRON_USERS_SOURCE}" ]]; then
        fail "La configuration source des utilisateurs Cron est invalide."
    fi
    if [[ ! -f "${CRON_USERS_DENY_SOURCE}" || -L "${CRON_USERS_DENY_SOURCE}" ]]; then
        fail "La configuration source des exclusions Cron est invalide."
    fi
    if [[ ! -f "${SERVICES_SOURCE}" || -L "${SERVICES_SOURCE}" ]]; then
        fail "La configuration source des services est invalide."
    fi
    if [[ ! -f "${APACHE_PROFILE_SOURCE}" || -L "${APACHE_PROFILE_SOURCE}" ]]; then
        fail "La configuration source Apache est invalide."
    fi
    if [[ ! -f "${LOGS_POLICY_SOURCE}" || -L "${LOGS_POLICY_SOURCE}" ]]; then
        fail "La configuration source des journaux est invalide."
    fi
    if [[ ! -f "${FAIL2BAN_PROFILE_SOURCE}" || -L "${FAIL2BAN_PROFILE_SOURCE}" ]]; then
        fail "La configuration source Fail2ban est invalide."
    fi
    if [[ ! -f "${TOR_PROFILE_SOURCE}" || -L "${TOR_PROFILE_SOURCE}" ]]; then
        fail "La configuration source Tor est invalide."
    fi
    if [[ ! -f "${CRON_PROFILE_SOURCE}" || -L "${CRON_PROFILE_SOURCE}" ]]; then
        fail "La configuration source Cron est invalide."
    fi
    if [[ ! -f "${STORAGE_PROFILE_SOURCE}" || -L "${STORAGE_PROFILE_SOURCE}" ]]; then
        fail "La configuration source du stockage est invalide."
    fi
    if [[ ! -f "${COMPOSER_PROFILE_SOURCE}" || -L "${COMPOSER_PROFILE_SOURCE}" ]]; then
        fail "La configuration source de Composer est invalide."
    fi
    [[ -d "${MIGRATIONS_SOURCE}" && ! -L "${MIGRATIONS_SOURCE}" ]] \
        || fail "Le répertoire des migrations SQLite est invalide."

    local unformatted=""
    unformatted="$(
        cd -- "${SCRIPT_DIR}"
        "${FIND_COMMAND}" . -type f -name '*.go' -print0 \
            | "${XARGS_COMMAND}" -0 "${GOFMT_COMMAND}" -l
    )"

    if [[ -n "${unformatted}" ]]; then
        fail "Des fichiers Go n’étaient pas formatés : ${unformatted}"
    fi

    (
        cd -- "${SCRIPT_DIR}"
        "${GO_COMMAND}" test ./...
        "${GO_COMMAND}" vet ./...
    )

    require_command "${VISUDO_COMMAND}" "visudo"
    "${VISUDO_COMMAND}" -cf \
        "${SCRIPT_DIR}/deploy/aegisadmin-system-go.sudoers"

    "${PYTHON_COMMAND}" -c \
        'import pathlib; compile(pathlib.Path(__import__("sys").argv[1]).read_bytes(), __import__("sys").argv[1], "exec")' \
        "${CRON_RUNNER_SOURCE}"
    "${PYTHON_COMMAND}" -c \
        'import pathlib; compile(pathlib.Path(__import__("sys").argv[1]).read_bytes(), __import__("sys").argv[1], "exec")' \
        "${CERTBOT_RUNNER_SOURCE}"

    printf '%s\n' "Sources AegisAdmin Go validées."
}

build_binaries()
{
    local output_directory="${1}"
    local build_version=""
    local linker_flags=""

    build_version="$(<"${APP_ROOT}/VERSION")"
    [[ "${build_version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] \
        || fail "La version du projet est invalide : ${build_version}" 2
    linker_flags="-X aegisadmin/backend/internal/buildinfo.Version=${build_version} -X aegisadmin/backend/internal/domain/configuration.defaultSnapshotDir=/var/lib/aegisadmin/configuration/snapshots -X aegisadmin/backend/internal/domain/cron.runnerPath=${CRON_RUNNER_TARGET} -X aegisadmin/backend/internal/domain/certbot.runnerPath=${CERTBOT_RUNNER_TARGET} -X aegisadmin/backend/internal/domain/firewall.goCLI=${CLI_TARGET}"

    (
        cd -- "${SCRIPT_DIR}"
        "${GO_COMMAND}" build \
            -trimpath \
            -ldflags "${linker_flags}" \
            -o "${output_directory}/aegisadmin-daemon" \
            ./cmd/aegisadmin-daemon
        "${GO_COMMAND}" build \
            -trimpath \
            -ldflags "${linker_flags}" \
            -o "${output_directory}/aegisadmin-system-go" \
            ./cmd/aegisadmin-system-go
        "${GO_COMMAND}" build \
            -trimpath \
            -ldflags "${linker_flags}" \
            -o "${output_directory}/aegisadmin-web" \
            ./cmd/aegisadmin-web
        "${GO_COMMAND}" build \
            -trimpath \
            -ldflags "${linker_flags}" \
            -o "${output_directory}/aegisadmin-admin" \
            ./cmd/aegisadmin-admin
        "${GO_COMMAND}" build \
            -trimpath \
            -ldflags "${linker_flags}" \
            -o "${output_directory}/aegisadmin-updater" \
            ./cmd/aegisadmin-updater
        "${GO_COMMAND}" build \
            -trimpath \
            -ldflags "${linker_flags}" \
            -o "${output_directory}/aegisadmin-backup" \
            ./cmd/aegisadmin-backup
    )
}

wait_for_socket()
{
    local attempt=0

    for (( attempt = 1; attempt <= 50; attempt++ )); do
        if [[ -S "/run/aegisadmin-system/backend.sock" ]]; then
            return 0
        fi

        "${SLEEP_COMMAND}" 0.1
    done

    fail \
        "Le socket du backend Go n’a pas été créé après le démarrage du service." \
        10
}

require_stable_service()
{
    local service="${1}"

    if ! "${SYSTEMCTL_COMMAND}" is-active --quiet "${service}"; then
        fail "Le service ${service} n’est pas actif après l’installation." 10
    fi
    "${SLEEP_COMMAND}" 1
    if ! "${SYSTEMCTL_COMMAND}" is-active --quiet "${service}"; then
        fail "Le service ${service} est retombé juste après son démarrage." 10
    fi
}

verify_installation()
{
    require_root
    discover_verify_commands

    [[ -x "${DAEMON_TARGET}" ]] \
        || fail "Le backend système installé est absent ou non exécutable : ${DAEMON_TARGET}" 10
    [[ -x "${WEB_TARGET}" ]] \
        || fail "Le serveur web installé est absent ou non exécutable : ${WEB_TARGET}" 10
    [[ -x "${UPDATER_TARGET}" ]] \
        || fail "L’exécuteur de mises à jour installé est absent : ${UPDATER_TARGET}" 10
    [[ -x "${BACKUP_TARGET}" ]] \
        || fail "L’exécuteur de sauvegarde installé est absent : ${BACKUP_TARGET}" 10
    [[ -f "${UPDATER_UNIT_TARGET}" ]] \
        || fail "L’unité indépendante de mise à jour est absente : ${UPDATER_UNIT_TARGET}" 10
    [[ -S "/run/aegisadmin-system/backend.sock" ]] \
        || fail "Le socket du backend système est absent." 10

    if ! "${RUNUSER_COMMAND}" -u aegisadmin -g "${WEB_GROUP}" -- test -x "${WEB_TARGET}"; then
        fail "Le compte aegisadmin ne peut pas exécuter le serveur web." 10
    fi
    for readable in \
        "${TLS_DIRECTORY}/admin-local.crt" \
        "${TLS_DIRECTORY}/admin-local.key" \
        "${APPLICATION_DATABASE_FILE}"; do
        if ! "${RUNUSER_COMMAND}" -u aegisadmin -g "${WEB_GROUP}" -- test -r "${readable}"; then
            fail "Le compte aegisadmin ne peut pas lire ${readable}." 10
        fi
    done

    require_stable_service aegisadmin-system.service
    require_stable_service aegisadmin-web.service
    printf '%s\n' "Installation AegisAdmin vérifiée : backend et interface web opérationnels."
}

cleanup_build_directory()
{
    if [[ -n "${BUILD_DIRECTORY}" && -d "${BUILD_DIRECTORY}" ]]; then
        "${RM_COMMAND}" -rf -- "${BUILD_DIRECTORY}"
    fi
}

remove_legacy_sources()
{
	local target=""
	for target in \
		"${APP_ROOT}/app" "${APP_ROOT}/bin" "${APP_ROOT}/config" \
		"${APP_ROOT}/resources" "${APP_ROOT}/routes" "${APP_ROOT}/tests" \
		"${APP_ROOT}/vendor" "${APP_ROOT}/backend/commands"; do
		if [[ -d "${target}" && ! -L "${target}" ]]; then
			"${RM_COMMAND}" -rf -- "${target}"
		fi
	done
	"${RM_COMMAND}" -f -- \
		"${APP_ROOT}/bootstrap.php" "${APP_ROOT}/composer.json" \
		"${APP_ROOT}/composer.lock" "${APP_ROOT}/default.php" \
		"${APP_ROOT}/footer.php" "${APP_ROOT}/sidebar.php" \
		"${APP_ROOT}/topbar.php" "${APP_ROOT}/public/index.php" \
		"${APP_ROOT}/public/info.php" "${APP_ROOT}/backend/aegisadmin-system" \
		"${APP_ROOT}/backend/install.sh"
}

install_backend()
{
    local web_user="${1}"

    require_root
    ensure_go_for_install
    discover_install_commands
    require_command "${INSTALL_COMMAND}" "install"
    require_command "${SLEEP_COMMAND}" "sleep"
    require_command "${SYSTEMCTL_COMMAND}" "systemctl"
    require_command "${GETENT_COMMAND}" "getent"
    require_command "${GROUPADD_COMMAND}" "groupadd"
    require_command "${USERMOD_COMMAND}" "usermod"
    require_command "${AWK_COMMAND}" "awk"
    require_command "${CHOWN_COMMAND}" "chown"
    require_command "${CHMOD_COMMAND}" "chmod"
    web_user="$(resolve_web_user "${web_user}")"
    if ! "${SYSTEMCTL_COMMAND}" cat aegisadmin-web.service >/dev/null 2>&1; then
        fail \
            "Le service aegisadmin-web.service est absent. Installez d’abord le paquet Debian AegisAdmin." \
            4
    fi
    validate_sources

    BUILD_DIRECTORY="$(
        "${MKTEMP_COMMAND}" -d /tmp/aegisadmin-go-build.XXXXXXXX
    )"
    trap cleanup_build_directory EXIT

    build_binaries "${BUILD_DIRECTORY}"
    configure_web_identity "${web_user}"
    configure_application_storage

    "${INSTALL_COMMAND}" -d -o root -g root -m 0750 \
        "$(dirname -- "${DAEMON_TARGET}")"
    "${INSTALL_COMMAND}" -d -o root -g root -m 0755 \
        "$(dirname -- "${CLI_TARGET}")"
	"${INSTALL_COMMAND}" -d -o root -g root -m 0755 "${MIGRATIONS_TARGET}"
    "${INSTALL_COMMAND}" -d -o root -g root -m 0755 \
        "$(dirname -- "${WEB_TARGET}")"
    "${INSTALL_COMMAND}" -d -o root -g root -m 0755 \
        "${DOCUMENTATION_DIRECTORY}"
    "${INSTALL_COMMAND}" -d -o root -g root -m 0700 \
        "${APACHE_BACKUP_DIRECTORY}"
    "${INSTALL_COMMAND}" -d -o root -g root -m 0700 \
        "${CRON_BACKUP_DIRECTORY}"
    "${INSTALL_COMMAND}" -d -o root -g root -m 0755 \
        "$(dirname -- "${CRON_RUNNER_TARGET}")"
    "${INSTALL_COMMAND}" -d -o root -g "${WEB_GROUP}" -m 0750 \
        "${SYSTEM_CONFIG_DIRECTORY}" "${TLS_DIRECTORY}"
    "${INSTALL_COMMAND}" -d -o root -g root -m 0700 \
        "${SYSTEM_CONFIG_DIRECTORY}/backup-tasks" \
        "${SYSTEM_CONFIG_DIRECTORY}/backup-ssh" \
        "/var/backups/aegisadmin"
    "${INSTALL_COMMAND}" -d -o root -g "${WEB_GROUP}" -m 0750 \
        "${APPLICATION_UPDATE_DIRECTORY}" "${APPLICATION_UPDATE_DIRECTORY}/jobs"

    "${INSTALL_COMMAND}" -o root -g root -m 0750 \
        "${BUILD_DIRECTORY}/aegisadmin-daemon" \
        "${DAEMON_TARGET}"
    "${INSTALL_COMMAND}" -o root -g root -m 0750 \
        "${BUILD_DIRECTORY}/aegisadmin-system-go" \
        "${CLI_TARGET}"
    "${INSTALL_COMMAND}" -o root -g root -m 0750 \
        "${BUILD_DIRECTORY}/aegisadmin-admin" \
        "${ADMIN_TARGET}"
    "${INSTALL_COMMAND}" -o root -g root -m 0755 \
        "${BUILD_DIRECTORY}/aegisadmin-web" \
        "${WEB_TARGET}"
    "${INSTALL_COMMAND}" -o root -g root -m 0750 \
        "${BUILD_DIRECTORY}/aegisadmin-updater" \
        "${UPDATER_TARGET}"
    "${INSTALL_COMMAND}" -o root -g root -m 0750 \
        "${BUILD_DIRECTORY}/aegisadmin-backup" \
        "${BACKUP_TARGET}"
	if ! "${RUNUSER_COMMAND}" -u aegisadmin -g "${WEB_GROUP}" -- test -x "${WEB_TARGET}"; then
		fail \
			"Le compte aegisadmin ne peut pas exécuter ${WEB_TARGET}. Vérifiez les droits de ses répertoires parents." \
			5
	fi
    "${INSTALL_COMMAND}" -o root -g root -m 0644 \
        "${SCRIPT_DIR}/deploy/aegisadmin-daemon.service" \
        "${UNIT_TARGET}"
    "${INSTALL_COMMAND}" -o root -g root -m 0644 \
        "${SCRIPT_DIR}/deploy/aegisadmin-updater@.service" \
        "${UPDATER_UNIT_TARGET}"
    "${INSTALL_COMMAND}" -o root -g root -m 0440 \
        "${SCRIPT_DIR}/deploy/aegisadmin-system-go.sudoers" \
        "${SUDOERS_TARGET}"
    "${INSTALL_COMMAND}" -o root -g root -m 0644 \
        "${SCRIPT_DIR}/DEPLOYMENT.md" \
        "${DOCUMENTATION_TARGET}"
    "${INSTALL_COMMAND}" -o root -g root -m 0750 \
        "${CRON_RUNNER_SOURCE}" \
        "${CRON_RUNNER_TARGET}"
    "${INSTALL_COMMAND}" -o root -g root -m 0750 \
        "${CERTBOT_RUNNER_SOURCE}" \
        "${CERTBOT_RUNNER_TARGET}"
	"${INSTALL_COMMAND}" -o root -g root -m 0644 \
		"${MIGRATIONS_SOURCE}"/*.sql "${MIGRATIONS_TARGET}/"

	"${ADMIN_TARGET}" --database "${APPLICATION_DATABASE_FILE}" \
		--migrations "${MIGRATIONS_TARGET}" migrate
	if "${SYSTEMCTL_COMMAND}" is-active --quiet mysql.service || \
	   "${SYSTEMCTL_COMMAND}" is-active --quiet mariadb.service || \
	   "${SYSTEMCTL_COMMAND}" is-active --quiet mysqld.service; then
		"${ADMIN_TARGET}" configure-mysql-auto
	fi
	configure_application_storage

    if [[ ! -e "${CRON_USERS_TARGET}" ]]; then
        "${INSTALL_COMMAND}" -o root -g root -m 0640 \
            "${CRON_USERS_SOURCE}" \
            "${CRON_USERS_TARGET}"
    fi
    if [[ ! -e "${CRON_USERS_DENY_TARGET}" ]]; then
        "${INSTALL_COMMAND}" -o root -g root -m 0640 \
            "${CRON_USERS_DENY_SOURCE}" \
            "${CRON_USERS_DENY_TARGET}"
    fi
    if [[ ! -e "${SERVICES_TARGET}" ]]; then
        "${INSTALL_COMMAND}" -o root -g root -m 0640 \
            "${SERVICES_SOURCE}" \
            "${SERVICES_TARGET}"
    fi
    if [[ ! -e "${APACHE_PROFILE_TARGET}" ]]; then
        "${INSTALL_COMMAND}" -o root -g root -m 0640 \
            "${APACHE_PROFILE_SOURCE}" \
            "${APACHE_PROFILE_TARGET}"
    fi
    if [[ ! -e "${LOGS_POLICY_TARGET}" ]]; then
        "${INSTALL_COMMAND}" -o root -g root -m 0640 \
            "${LOGS_POLICY_SOURCE}" \
            "${LOGS_POLICY_TARGET}"
    fi
    if [[ ! -e "${FAIL2BAN_PROFILE_TARGET}" ]]; then
        "${INSTALL_COMMAND}" -o root -g root -m 0640 \
            "${FAIL2BAN_PROFILE_SOURCE}" \
            "${FAIL2BAN_PROFILE_TARGET}"
    fi
    if [[ ! -e "${TOR_PROFILE_TARGET}" ]]; then
        "${INSTALL_COMMAND}" -o root -g root -m 0640 \
            "${TOR_PROFILE_SOURCE}" \
            "${TOR_PROFILE_TARGET}"
    fi
    if [[ ! -e "${CRON_PROFILE_TARGET}" ]]; then
        "${INSTALL_COMMAND}" -o root -g root -m 0640 \
            "${CRON_PROFILE_SOURCE}" \
            "${CRON_PROFILE_TARGET}"
    fi
    if [[ ! -e "${STORAGE_PROFILE_TARGET}" ]]; then
        "${INSTALL_COMMAND}" -o root -g root -m 0640 \
            "${STORAGE_PROFILE_SOURCE}" \
            "${STORAGE_PROFILE_TARGET}"
    fi
    if [[ ! -e "${COMPOSER_PROFILE_TARGET}" ]]; then
        "${INSTALL_COMMAND}" -o root -g root -m 0640 \
            "${COMPOSER_PROFILE_SOURCE}" \
            "${COMPOSER_PROFILE_TARGET}"
    fi

    if "${SYSTEMCTL_COMMAND}" cat aegisadmin-daemon.service >/dev/null 2>&1; then
        "${SYSTEMCTL_COMMAND}" disable --now aegisadmin-daemon.service \
            >/dev/null 2>&1 || true
    fi
    "${RM_COMMAND}" -f -- \
        "${LEGACY_DAEMON_TARGET}" "${LEGACY_CLI_TARGET}" \
        "${LEGACY_ADMIN_TARGET}" "${LEGACY_UNIT_TARGET}" \
        "${LEGACY_SUDOERS_TARGET}" "${LEGACY_SYSTEM_OVERRIDE_TARGET}"

    "${SYSTEMCTL_COMMAND}" daemon-reload
    "${SYSTEMCTL_COMMAND}" enable aegisadmin-system.service
    "${SYSTEMCTL_COMMAND}" restart aegisadmin-system.service
    wait_for_socket
	if [[ -f "/etc/aegisadmin-system/admin-web" && ! -L "/etc/aegisadmin-system/admin-web" ]]; then
		"${CLI_TARGET}" admin-access reconcile-package >/dev/null
	fi

    "${SYSTEMCTL_COMMAND}" restart aegisadmin-web.service
	verify_installation

	"${RM_COMMAND}" -f -- "${LEGACY_BACKEND_TARGET}"
	if [[ -d "${LEGACY_COMMANDS_TARGET}" && ! -L "${LEGACY_COMMANDS_TARGET}" ]]; then
		"${RM_COMMAND}" -rf -- "${LEGACY_COMMANDS_TARGET}"
	fi
	remove_legacy_sources
	if [[ -d "/etc/aegisadmin" && ! -L "/etc/aegisadmin" ]]; then
		"${RM_COMMAND}" -rf -- "/etc/aegisadmin"
	fi

    printf '%s\n' \
        "Backend Go principal installé." \
        "Interface web Go installée." \
        "Frontend PHP et backend Bash historique retirés."
}

remove_backend()
{
    require_root
    if command -v dpkg-query >/dev/null 2>&1 && \
       dpkg-query -W -f='${db:Status-Status}' aegisadmin 2>/dev/null | grep -qx installed; then
        fail \
            "AegisAdmin appartient au paquet Debian. Utilisez : sudo apt remove aegisadmin" \
            2
    fi
    discover_remove_commands
    require_command "${RM_COMMAND}" "rm"
    require_command "${SYSTEMCTL_COMMAND}" "systemctl"

    "${SYSTEMCTL_COMMAND}" disable --now aegisadmin-system.service \
        >/dev/null 2>&1 || true
    "${SYSTEMCTL_COMMAND}" stop 'aegisadmin-updater@*.service' \
        >/dev/null 2>&1 || true

    "${RM_COMMAND}" -f -- \
        "${DAEMON_TARGET}" \
        "${CLI_TARGET}" \
		"${ADMIN_TARGET}" \
        "${UPDATER_TARGET}" \
        "${BACKUP_TARGET}" \
        "${UNIT_TARGET}" \
        "${UPDATER_UNIT_TARGET}" \
        "${SUDOERS_TARGET}" \
        "${DOCUMENTATION_TARGET}"

    "${SYSTEMCTL_COMMAND}" daemon-reload
    "${SYSTEMCTL_COMMAND}" reset-failed aegisadmin-system.service \
        >/dev/null 2>&1 || true

    printf '%s\n' "Backend Go retiré."
}

main()
{
    if (( $# < 1 )); then
        usage >&2
        exit 2
    fi

    local action="${1}"
    shift

    case "${action}" in
        check)
            require_no_arguments "$@"
            discover_validation_commands
            validate_sources
            ;;
        verify)
            require_no_arguments "$@"
            verify_installation
            ;;
        install)
            parse_install_arguments "$@"
            install_backend "${INSTALL_WEB_USER}"
            ;;
        remove)
            require_no_arguments "$@"
            remove_backend
            ;;
        help|--help|-h)
            require_no_arguments "$@"
            usage
            ;;
        *)
            usage >&2
            fail "Commande inconnue : ${action}" 2
            ;;
    esac
}

main "$@"
