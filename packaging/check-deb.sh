#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'
umask 077

fail() { printf 'ERREUR : %s\n' "$1" >&2; exit "${2:-1}"; }

if (( $# != 1 )); then
    printf 'Usage : %s CHEMIN_DU_PAQUET.deb\n' "$0" >&2
    exit 2
fi

readonly PACKAGE_FILE="$1"
readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
readonly PROJECT_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd -P)"
[[ -f "${PACKAGE_FILE}" && ! -L "${PACKAGE_FILE}" ]] \
    || fail "Le paquet est absent ou invalide : ${PACKAGE_FILE}" 2

for command in dpkg-deb find grep mkdir mktemp rm sort stat tr wc; do
    command -v "${command}" >/dev/null || fail "Commande de contrôle absente : ${command}" 3
done

WORK_DIRECTORY="$(mktemp -d /tmp/aegisadmin-deb-check.XXXXXXXX)"
trap 'rm -rf -- "${WORK_DIRECTORY}"' EXIT
readonly ROOT="${WORK_DIRECTORY}/root"
readonly CONTROL="${WORK_DIRECTORY}/control"
mkdir -p "${ROOT}" "${CONTROL}"
dpkg-deb --extract "${PACKAGE_FILE}" "${ROOT}"
dpkg-deb --control "${PACKAGE_FILE}" "${CONTROL}"

readonly PROJECT_VERSION="$(tr -d '[:space:]' < "${PROJECT_ROOT}/VERSION")"
readonly PACKAGE_NAME="$(dpkg-deb -f "${PACKAGE_FILE}" Package)"
readonly PACKAGE_VERSION="$(dpkg-deb -f "${PACKAGE_FILE}" Version)"
[[ "${PACKAGE_NAME}" == aegisadmin ]] || fail "Nom de paquet inattendu : ${PACKAGE_NAME}"
[[ "${PACKAGE_VERSION}" == "1:${PROJECT_VERSION}" ]] \
    || fail "Version du paquet inattendue : ${PACKAGE_VERSION} (attendu : 1:${PROJECT_VERSION})"

for script in postinst prerm postrm; do
    [[ -x "${CONTROL}/${script}" ]] || fail "Script Debian absent ou non exécutable : ${script}"
    /bin/sh -n "${CONTROL}/${script}"
done

required_files=(
    usr/bin/aegisadmin
    usr/bin/aegisadmin-system-go
    usr/libexec/aegisadmin/aegisadmin-admin
    usr/libexec/aegisadmin/aegisadmin-daemon
    usr/libexec/aegisadmin/aegisadmin-web
    usr/libexec/aegisadmin/aegisadmin-updater
    usr/libexec/aegisadmin/aegisadmin-backup
    usr/libexec/aegisadmin/certbot-runner.py
    usr/libexec/aegisadmin/cron-runner.py
    lib/systemd/system/aegisadmin-system.service
    lib/systemd/system/aegisadmin-web.service
    lib/systemd/system/aegisadmin-updater@.service
    etc/sudoers.d/aegisadmin
    usr/share/aegisadmin/VERSION
    usr/share/aegisadmin/defaults/admin-web
    usr/share/aegisadmin/defaults/aegisadmin-admin.conf
    usr/share/doc/aegisadmin/operations.md
    usr/share/doc/aegisadmin/admin-https-access.md
    usr/share/doc/aegisadmin/root-password-recovery.md
    usr/share/doc/aegisadmin/debian-lifecycle-tests.md
    usr/share/doc/aegisadmin/apt-repository.md
)
for path in "${required_files[@]}"; do
    [[ -f "${ROOT}/${path}" && ! -L "${ROOT}/${path}" ]] \
        || fail "Fichier obligatoire absent du paquet : /${path}"
done

[[ "$(stat -c '%a' "${ROOT}/usr/libexec/aegisadmin/aegisadmin-web")" == 755 ]] \
    || fail "Le serveur web du paquet n’est pas exécutable par son compte système."
[[ "$(stat -c '%a' "${ROOT}/usr/libexec/aegisadmin/aegisadmin-updater")" == 750 ]] \
    || fail "Les droits de l’exécuteur de mises à jour ne sont pas 0750."
[[ "$(stat -c '%a' "${ROOT}/usr/libexec/aegisadmin/aegisadmin-backup")" == 750 ]] \
    || fail "Les droits de l’exécuteur de sauvegarde ne sont pas 0750."
grep -q 'mysql-setup' "${ROOT}/usr/bin/aegisadmin" \
    || fail "La commande de configuration MySQL est absente du lanceur principal."
grep -q 'configure-mysql-auto' "${CONTROL}/postinst" \
    || fail "L’installation ne tente pas de préparer le compte MySQL de supervision."
grep -q '^ExecStart=/usr/libexec/aegisadmin/aegisadmin-updater --job %i ' \
    "${ROOT}/lib/systemd/system/aegisadmin-updater@.service" \
    || fail "L’unité systemd ne lance pas l’exécuteur de mises à jour attendu."
grep -q -- '--sessions /var/lib/aegisadmin/sessions/sessions.json' \
    "${ROOT}/lib/systemd/system/aegisadmin-web.service" \
    || fail "L’unité web ne configure pas le stockage persistant des sessions."
grep -q -- '--secret-key /var/lib/aegisadmin/secrets/settings.key' \
    "${ROOT}/lib/systemd/system/aegisadmin-web.service" \
    || fail "L’unité web ne configure pas la clé des secrets applicatifs."
grep -q -- '--allow-from ${AEGISADMIN_WEB_ALLOW_FROM}' \
    "${ROOT}/lib/systemd/system/aegisadmin-web.service" \
    || fail "L’unité web ne transmet pas la restriction réseau au serveur Go."
grep -q '^StartLimitIntervalSec=60s$' "${ROOT}/lib/systemd/system/aegisadmin-web.service" && \
grep -q '^StartLimitBurst=5$' "${ROOT}/lib/systemd/system/aegisadmin-web.service" \
    || fail "L’unité web ne limite pas les boucles de redémarrage."
grep -q 'normalize_database_permissions' "${ROOT}/usr/bin/aegisadmin" && \
grep -q 'systemctl stop aegisadmin-web.service' "${ROOT}/usr/bin/aegisadmin" \
    || fail "Le lanceur ne sécurise pas les maintenances SQLite."
if grep -q '^Depends:.*apache2' "${CONTROL}/control"; then
    fail "Apache ne doit pas être une dépendance obligatoire du paquet."
fi
grep -q '^ReadWritePaths=/var/lib/aegisadmin/database /var/lib/aegisadmin/sessions /var/lib/aegisadmin/secrets$' \
    "${ROOT}/lib/systemd/system/aegisadmin-web.service" \
    || fail "L’unité web n’autorise pas uniquement les stockages attendus."
[[ "$(stat -c '%a' "${ROOT}/etc/sudoers.d/aegisadmin")" == 440 ]] \
    || fail "Les droits sudoers du paquet ne sont pas 0440."

[[ ! -e "${ROOT}/var/lib/aegisadmin" ]] \
    || fail "Le paquet ne doit contenir aucune donnée persistante sous /var/lib/aegisadmin."
[[ ! -e "${ROOT}/etc/aegisadmin-system/tls/admin-local.crt" && \
   ! -e "${ROOT}/etc/aegisadmin-system/tls/admin-local.key" ]] \
    || fail "La paire TLS locale ne doit jamais être embarquée dans le paquet."
[[ ! -e "${ROOT}/etc/aegisadmin-system/admin-web" && \
   ! -e "${ROOT}/etc/apache2/sites-available/aegisadmin-admin.conf" && \
   ! -e "${ROOT}/etc/aegisadmin-system/mysql-client.cnf" ]] \
    || fail "Les configurations dynamiques doivent être créées depuis leurs modèles, pas embarquées comme conffiles."

sort "${CONTROL}/conffiles" > "${WORK_DIRECTORY}/conffiles.sorted"
[[ "$(sort -u "${CONTROL}/conffiles" | wc -l)" -eq "$(wc -l < "${CONTROL}/conffiles")" ]] \
    || fail "La liste des fichiers de configuration Debian contient des doublons."
while IFS= read -r path; do
    [[ "${path}" == /* ]] || fail "Chemin de configuration non absolu : ${path}"
    [[ -f "${ROOT}${path}" && ! -L "${ROOT}${path}" ]] \
        || fail "Fichier de configuration déclaré mais absent : ${path}"
done < "${CONTROL}/conffiles"

if find "${ROOT}" -type f -perm /0002 -print -quit | grep -q .; then
    fail "Le paquet contient un fichier modifiable par tous."
fi

printf 'Paquet Debian AegisAdmin vérifié : %s (%s).\n' \
    "${PACKAGE_NAME}" "${PACKAGE_VERSION}"
