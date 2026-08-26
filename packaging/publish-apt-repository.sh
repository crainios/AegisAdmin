#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'
umask 022

fail() { printf 'ERREUR : %s\n' "$1" >&2; exit "${2:-1}"; }
usage() {
    printf '%s\n' \
        "Usage :" \
        "  sudo bash $0 publish --repository REPERTOIRE --release VERSION [--root REPERTOIRE]" \
        "  sudo bash $0 rollback --release VERSION [--root REPERTOIRE]" \
        "  sudo bash $0 check [--root REPERTOIRE]" \
        "" \
        "Le répertoire racine par défaut est /var/www/packages.aegisadmin.fr."
}

(( $# >= 1 )) || { usage >&2; exit 2; }
case "$1" in
    --help|-h) usage; exit 0 ;;
esac
MODE="$1"
shift
REPOSITORY=""
RELEASE=""
ROOT_DIRECTORY="/var/www/packages.aegisadmin.fr"

while (( $# > 0 )); do
    case "$1" in
        --repository) (( $# >= 2 )) || fail "Répertoire du dépôt absent." 2; REPOSITORY="$2"; shift 2 ;;
        --release) (( $# >= 2 )) || fail "Version de publication absente." 2; RELEASE="$2"; shift 2 ;;
        --root) (( $# >= 2 )) || fail "Répertoire racine absent." 2; ROOT_DIRECTORY="$2"; shift 2 ;;
        --help|-h) usage; exit 0 ;;
        *) usage >&2; fail "Option inconnue : $1" 2 ;;
    esac
done

[[ "${MODE}" == publish || "${MODE}" == rollback || "${MODE}" == check ]] \
    || { usage >&2; fail "Action inconnue : ${MODE}" 2; }
(( EUID == 0 )) || fail "La publication doit être exécutée avec sudo." 3
[[ "${ROOT_DIRECTORY}" == /* && "${ROOT_DIRECTORY}" != / ]] \
    || fail "Le répertoire racine doit être un chemin absolu dédié." 2
[[ ! -L "${ROOT_DIRECTORY}" ]] || fail "Le répertoire racine ne doit pas être un lien symbolique."

for command in basename chown cp find install ln mktemp mv readlink rm; do
    command -v "${command}" >/dev/null || fail "Commande de publication absente : ${command}" 3
done

SCRIPT_DIRECTORY="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
CHECK_SCRIPT="${SCRIPT_DIRECTORY}/check-apt-repository.sh"
[[ -f "${CHECK_SCRIPT}" ]] || fail "Contrôleur du dépôt introuvable : ${CHECK_SCRIPT}"

install -d -o root -g root -m 0755 "${ROOT_DIRECTORY}" "${ROOT_DIRECTORY}/releases"
RELEASES_DIRECTORY="${ROOT_DIRECTORY}/releases"
CURRENT_LINK="${ROOT_DIRECTORY}/current"
[[ ! -L "${RELEASES_DIRECTORY}" ]] || fail "Le répertoire des publications ne doit pas être un lien symbolique."

activate_release() {
    local release_name="$1"
    local target="${RELEASES_DIRECTORY}/${release_name}"
    local temporary_link="${ROOT_DIRECTORY}/.current.$$.tmp"

    [[ -d "${target}" && ! -L "${target}" ]] || fail "Publication introuvable : ${release_name}"
    bash "${CHECK_SCRIPT}" "${target}"
    ln -s "releases/${release_name}" "${temporary_link}"
    mv -Tf -- "${temporary_link}" "${CURRENT_LINK}"
    printf 'Publication APT active : %s\n' "${release_name}"
}

case "${MODE}" in
    publish)
        [[ -d "${REPOSITORY}" && ! -L "${REPOSITORY}" ]] || fail "Répertoire du dépôt invalide."
        [[ -z "$(find "${REPOSITORY}" -type l -print -quit)" ]] \
            || fail "Le dépôt source ne doit contenir aucun lien symbolique."
        [[ "${RELEASE}" =~ ^[0-9A-Za-z][0-9A-Za-z.+:~-]*$ ]] || fail "Nom de publication invalide."
        TARGET="${RELEASES_DIRECTORY}/${RELEASE}"
        [[ ! -e "${TARGET}" ]] || fail "La publication ${RELEASE} existe déjà."
        bash "${CHECK_SCRIPT}" "${REPOSITORY}"

        STAGING="$(mktemp -d "${RELEASES_DIRECTORY}/.incoming.XXXXXXXX")"
        trap 'rm -rf -- "${STAGING}"' EXIT
        cp -a -- "${REPOSITORY}/." "${STAGING}/"
        chown -R root:root "${STAGING}"
        find "${STAGING}" -type d -exec chmod 0755 {} +
        find "${STAGING}" -type f -exec chmod 0644 {} +
        bash "${CHECK_SCRIPT}" "${STAGING}"
        mv -- "${STAGING}" "${TARGET}"
        trap - EXIT
        activate_release "${RELEASE}"
        ;;
    rollback)
        [[ "${RELEASE}" =~ ^[0-9A-Za-z][0-9A-Za-z.+:~-]*$ ]] || fail "Nom de publication invalide."
        activate_release "${RELEASE}"
        ;;
    check)
        [[ -z "${REPOSITORY}" && -z "${RELEASE}" ]] \
            || fail "L’action check n’accepte ni --repository ni --release." 2
        [[ -L "${CURRENT_LINK}" ]] || fail "Aucune publication active."
        ACTIVE_DIRECTORY="$(readlink -f -- "${CURRENT_LINK}")"
        case "${ACTIVE_DIRECTORY}" in
            "${RELEASES_DIRECTORY}"/*) ;;
            *) fail "Le lien current sort du répertoire des publications." ;;
        esac
        bash "${CHECK_SCRIPT}" "${ACTIVE_DIRECTORY}"
        printf 'Publication APT contrôlée : %s\n' "$(basename -- "${ACTIVE_DIRECTORY}")"
        ;;
esac
