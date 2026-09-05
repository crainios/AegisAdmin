#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'
umask 022

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
readonly PROJECT_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd -P)"
readonly DEBIAN_SOURCE="${SCRIPT_DIR}/debian"

VERSION=""
PACKAGE_VERSION=""
ARCHITECTURE="$(dpkg --print-architecture)"
OUTPUT_DIRECTORY="${PROJECT_ROOT}/dist"
BUILD_DIRECTORY=""

fail() { printf 'ERREUR : %s\n' "$1" >&2; exit "${2:-1}"; }
cleanup() { [[ -z "${BUILD_DIRECTORY}" ]] || rm -rf -- "${BUILD_DIRECTORY}"; }
trap cleanup EXIT

usage() {
    printf '%s\n' \
        "Usage : ${0} [--arch amd64|arm64] [--output REPERTOIRE]"
}

while (( $# > 0 )); do
    case "$1" in
        --arch) (( $# >= 2 )) || fail 'Architecture absente.' 2; ARCHITECTURE="$2"; shift 2 ;;
        --output) (( $# >= 2 )) || fail 'Répertoire de sortie absent.' 2; OUTPUT_DIRECTORY="$2"; shift 2 ;;
        --help|-h) usage; exit 0 ;;
        *) usage >&2; fail "Option inconnue : $1" 2 ;;
    esac
done

[[ -r "${PROJECT_ROOT}/VERSION" ]] || fail 'Le fichier VERSION est absent ou illisible.'
VERSION="$(tr -d '[:space:]' < "${PROJECT_ROOT}/VERSION")"
[[ "${VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] \
    || fail 'VERSION doit respecter le format MAJEURE.MINEURE.CORRECTIF.' 2
PACKAGE_VERSION="1:${VERSION}"
case "${ARCHITECTURE}" in
    amd64) GO_ARCH=amd64 ;;
    arm64) GO_ARCH=arm64 ;;
    *) fail "Architecture non prise en charge : ${ARCHITECTURE}" 2 ;;
esac

for command in go rsync dpkg-deb sed install find touch; do
    command -v "${command}" >/dev/null || fail "Commande de construction absente : ${command}"
done
BUILD_DIRECTORY="$(mktemp -d /tmp/aegisadmin-deb.XXXXXXXX)"
readonly PACKAGE_ROOT="${BUILD_DIRECTORY}/root"
install -d "${PACKAGE_ROOT}/DEBIAN" \
    "${PACKAGE_ROOT}/usr/bin" \
    "${PACKAGE_ROOT}/usr/libexec/aegisadmin" \
    "${PACKAGE_ROOT}/usr/share/aegisadmin/migrations" \
    "${PACKAGE_ROOT}/usr/share/aegisadmin/web/public" \
    "${PACKAGE_ROOT}/usr/share/aegisadmin/defaults" \
    "${PACKAGE_ROOT}/usr/share/aegisadmin" \
    "${PACKAGE_ROOT}/usr/share/doc/aegisadmin" \
    "${PACKAGE_ROOT}/etc/aegisadmin-system" \
    "${PACKAGE_ROOT}/etc/sudoers.d" \
    "${PACKAGE_ROOT}/lib/systemd/system"

readonly LINKER_FLAGS="-s -w -buildid= -X aegisadmin/backend/internal/buildinfo.Version=${VERSION} -X aegisadmin/backend/internal/domain/configuration.defaultSnapshotDir=/var/lib/aegisadmin/configuration/snapshots -X aegisadmin/backend/internal/domain/cron.runnerPath=/usr/libexec/aegisadmin/cron-runner.py -X aegisadmin/backend/internal/domain/certbot.runnerPath=/usr/libexec/aegisadmin/certbot-runner.py -X aegisadmin/backend/internal/domain/firewall.goCLI=/usr/bin/aegisadmin-system-go"
(
    cd -- "${PROJECT_ROOT}/backend/go"
    CGO_ENABLED=0 GOOS=linux GOARCH="${GO_ARCH}" go build -trimpath -ldflags "${LINKER_FLAGS}" \
        -o "${PACKAGE_ROOT}/usr/libexec/aegisadmin/aegisadmin-daemon" ./cmd/aegisadmin-daemon
    CGO_ENABLED=0 GOOS=linux GOARCH="${GO_ARCH}" go build -trimpath -ldflags "${LINKER_FLAGS}" \
        -o "${PACKAGE_ROOT}/usr/bin/aegisadmin-system-go" ./cmd/aegisadmin-system-go
    CGO_ENABLED=0 GOOS=linux GOARCH="${GO_ARCH}" go build -trimpath -ldflags "${LINKER_FLAGS}" \
        -o "${PACKAGE_ROOT}/usr/libexec/aegisadmin/aegisadmin-web" ./cmd/aegisadmin-web
    CGO_ENABLED=0 GOOS=linux GOARCH="${GO_ARCH}" go build -trimpath -ldflags "${LINKER_FLAGS}" \
        -o "${PACKAGE_ROOT}/usr/libexec/aegisadmin/aegisadmin-admin" ./cmd/aegisadmin-admin
    CGO_ENABLED=0 GOOS=linux GOARCH="${GO_ARCH}" go build -trimpath -ldflags "${LINKER_FLAGS}" \
        -o "${PACKAGE_ROOT}/usr/libexec/aegisadmin/aegisadmin-updater" ./cmd/aegisadmin-updater
    CGO_ENABLED=0 GOOS=linux GOARCH="${GO_ARCH}" go build -trimpath -ldflags "${LINKER_FLAGS}" \
        -o "${PACKAGE_ROOT}/usr/libexec/aegisadmin/aegisadmin-backup" ./cmd/aegisadmin-backup
)

install -m 0644 "${PROJECT_ROOT}/VERSION" "${PACKAGE_ROOT}/usr/share/aegisadmin/VERSION"

rsync -a "${PROJECT_ROOT}/database/migrations/" "${PACKAGE_ROOT}/usr/share/aegisadmin/migrations/"
rsync -a "${PROJECT_ROOT}/public/assets/" "${PACKAGE_ROOT}/usr/share/aegisadmin/web/public/assets/"
rsync -a "${PROJECT_ROOT}/backend/config/" "${PACKAGE_ROOT}/etc/aegisadmin-system/"
install -m 0755 "${DEBIAN_SOURCE}/aegisadmin" "${PACKAGE_ROOT}/usr/bin/aegisadmin"
install -m 0750 "${PROJECT_ROOT}/backend/libexec/cron-runner.py" "${PACKAGE_ROOT}/usr/libexec/aegisadmin/cron-runner.py"
install -m 0750 "${PROJECT_ROOT}/backend/libexec/certbot-runner.py" "${PACKAGE_ROOT}/usr/libexec/aegisadmin/certbot-runner.py"
install -m 0644 "${DEBIAN_SOURCE}/aegisadmin-system.service" "${PACKAGE_ROOT}/lib/systemd/system/aegisadmin-system.service"
install -m 0644 "${DEBIAN_SOURCE}/aegisadmin-web.service" "${PACKAGE_ROOT}/lib/systemd/system/aegisadmin-web.service"
install -m 0644 "${DEBIAN_SOURCE}/aegisadmin-updater@.service" "${PACKAGE_ROOT}/lib/systemd/system/aegisadmin-updater@.service"
install -m 0640 "${DEBIAN_SOURCE}/web-server" "${PACKAGE_ROOT}/etc/aegisadmin-system/web-server"
install -m 0640 "${DEBIAN_SOURCE}/admin-web" "${PACKAGE_ROOT}/usr/share/aegisadmin/defaults/admin-web"
install -m 0644 "${DEBIAN_SOURCE}/aegisadmin-apache.conf" "${PACKAGE_ROOT}/usr/share/aegisadmin/defaults/aegisadmin-admin.conf"
install -m 0440 "${DEBIAN_SOURCE}/aegisadmin.sudoers" "${PACKAGE_ROOT}/etc/sudoers.d/aegisadmin"
install -m 0644 "${PROJECT_ROOT}/docs/en/debian-packaging.md" "${PACKAGE_ROOT}/usr/share/doc/aegisadmin/debian-packaging.md"
install -m 0644 "${PROJECT_ROOT}/docs/en/operations.md" "${PACKAGE_ROOT}/usr/share/doc/aegisadmin/operations.md"
install -m 0644 "${PROJECT_ROOT}/docs/en/admin-https-access.md" "${PACKAGE_ROOT}/usr/share/doc/aegisadmin/admin-https-access.md"
install -m 0644 "${PROJECT_ROOT}/docs/en/modules-and-permissions.md" "${PACKAGE_ROOT}/usr/share/doc/aegisadmin/modules-and-permissions.md"
install -m 0644 "${PROJECT_ROOT}/docs/en/root-password-recovery.md" "${PACKAGE_ROOT}/usr/share/doc/aegisadmin/root-password-recovery.md"
install -m 0644 "${PROJECT_ROOT}/docs/en/debian-lifecycle-tests.md" "${PACKAGE_ROOT}/usr/share/doc/aegisadmin/debian-lifecycle-tests.md"
install -m 0644 "${PROJECT_ROOT}/docs/en/apt-repository.md" "${PACKAGE_ROOT}/usr/share/doc/aegisadmin/apt-repository.md"
install -m 0644 "${PROJECT_ROOT}/docs/architecture/debian-packaging.md" "${PACKAGE_ROOT}/usr/share/doc/aegisadmin/debian-packaging.fr.md"
install -m 0644 "${PROJECT_ROOT}/docs/operations.md" "${PACKAGE_ROOT}/usr/share/doc/aegisadmin/operations.fr.md"
install -m 0644 "${PROJECT_ROOT}/docs/admin-https-access.md" "${PACKAGE_ROOT}/usr/share/doc/aegisadmin/admin-https-access.fr.md"
install -m 0644 "${PROJECT_ROOT}/docs/modules-and-permissions.md" "${PACKAGE_ROOT}/usr/share/doc/aegisadmin/modules-and-permissions.fr.md"
install -m 0644 "${PROJECT_ROOT}/docs/root-password-recovery.md" "${PACKAGE_ROOT}/usr/share/doc/aegisadmin/root-password-recovery.fr.md"
install -m 0644 "${PROJECT_ROOT}/docs/debian-lifecycle-tests.md" "${PACKAGE_ROOT}/usr/share/doc/aegisadmin/debian-lifecycle-tests.fr.md"
install -m 0644 "${PROJECT_ROOT}/docs/apt-repository.md" "${PACKAGE_ROOT}/usr/share/doc/aegisadmin/apt-repository.fr.md"
sed -E -i 's#\]\(\.\./([^)]*)\.md\)#](\1.fr.md)#g' \
    "${PACKAGE_ROOT}/usr/share/doc/aegisadmin/"*.md

for script in postinst prerm postrm; do
    /bin/sh -n "${DEBIAN_SOURCE}/${script}"
    install -m 0755 "${DEBIAN_SOURCE}/${script}" "${PACKAGE_ROOT}/DEBIAN/${script}"
done
install -m 0644 "${DEBIAN_SOURCE}/conffiles" "${PACKAGE_ROOT}/DEBIAN/conffiles"
sed -e "s/@VERSION@/${PACKAGE_VERSION}/g" -e "s/@ARCH@/${ARCHITECTURE}/g" \
    "${DEBIAN_SOURCE}/control.template" > "${PACKAGE_ROOT}/DEBIAN/control"

find "${PACKAGE_ROOT}" -type d -exec chmod 0755 {} +
find "${PACKAGE_ROOT}" -type f -exec chmod 0644 {} +
chmod 0755 "${PACKAGE_ROOT}/DEBIAN/postinst" "${PACKAGE_ROOT}/DEBIAN/prerm" "${PACKAGE_ROOT}/DEBIAN/postrm"
chmod 0755 "${PACKAGE_ROOT}/usr/bin/aegisadmin" "${PACKAGE_ROOT}/usr/bin/aegisadmin-system-go"
chmod 0750 "${PACKAGE_ROOT}/usr/libexec/aegisadmin/aegisadmin-daemon"
chmod 0755 "${PACKAGE_ROOT}/usr/libexec/aegisadmin/aegisadmin-web"
chmod 0750 "${PACKAGE_ROOT}/usr/libexec/aegisadmin/aegisadmin-admin"
chmod 0750 "${PACKAGE_ROOT}/usr/libexec/aegisadmin/aegisadmin-updater"
chmod 0750 "${PACKAGE_ROOT}/usr/libexec/aegisadmin/aegisadmin-backup"
chmod 0750 "${PACKAGE_ROOT}/usr/libexec/aegisadmin/cron-runner.py" "${PACKAGE_ROOT}/usr/libexec/aegisadmin/certbot-runner.py"
find "${PACKAGE_ROOT}/etc/aegisadmin-system" -maxdepth 1 -type f -exec chmod 0640 {} +
chmod 0440 "${PACKAGE_ROOT}/etc/sudoers.d/aegisadmin"

if command -v visudo >/dev/null; then
    visudo -cf "${PACKAGE_ROOT}/etc/sudoers.d/aegisadmin" >/dev/null
fi

readonly BUILD_EPOCH="${SOURCE_DATE_EPOCH:-$(git -C "${PROJECT_ROOT}" log -1 --format=%ct 2>/dev/null || date +%s)}"
find "${PACKAGE_ROOT}" -print0 | xargs -0 touch --no-dereference --date="@${BUILD_EPOCH}"
install -d "${OUTPUT_DIRECTORY}"
readonly PACKAGE_FILE="${OUTPUT_DIRECTORY}/aegisadmin_${VERSION}_${ARCHITECTURE}.deb"
dpkg-deb --build --root-owner-group "${PACKAGE_ROOT}" "${PACKAGE_FILE}"
/bin/bash "${SCRIPT_DIR}/check-deb.sh" "${PACKAGE_FILE}"
printf 'Paquet créé : %s\n' "${PACKAGE_FILE}"
