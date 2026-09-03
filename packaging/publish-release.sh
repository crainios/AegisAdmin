#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'
umask 077

fail() { printf 'ERREUR : %s\n' "$1" >&2; exit "${2:-1}"; }
usage() {
    printf '%s\n' \
        "Usage :" \
        "  bash $0 configure" \
        "  bash $0 publish" \
        "  bash $0 show-config" \
        "" \
        "La configuration ne contient jamais la phrase secrète GPG."
}

(( $# == 1 )) || { usage >&2; exit 2; }
MODE="$1"
if [[ "${MODE}" == "--help" || "${MODE}" == "-h" ]]; then
    usage
    exit 0
fi
[[ "${MODE}" == configure || "${MODE}" == publish || "${MODE}" == show-config ]] \
    || { usage >&2; fail "Action inconnue : ${MODE}" 2; }
(( EUID != 0 )) || fail "Exécutez cet outil avec votre compte habituel, sans sudo." 3

SCRIPT_DIRECTORY="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
PROJECT_ROOT="$(cd -- "${SCRIPT_DIRECTORY}/.." && pwd -P)"
CONFIG_ROOT="${XDG_CONFIG_HOME:-${HOME}/.config}/aegisadmin"
CONFIG_FILE="${AEGISADMIN_PUBLISH_CONFIG:-${CONFIG_ROOT}/publisher.conf}"

validate_configuration() {
    [[ "${SIGNING_KEY:-}" =~ ^[A-Fa-f0-9]{16,64}$ ]] \
        || fail "L’empreinte GPG enregistrée est invalide."
    [[ "${REMOTE_TARGET:-}" =~ ^[A-Za-z_][A-Za-z0-9._-]*@[A-Za-z0-9][A-Za-z0-9.-]*$ ]] \
        || fail "La destination SSH enregistrée est invalide."
    [[ "${REMOTE_ROOT:-}" =~ ^/[A-Za-z0-9._/-]+$ && "${REMOTE_ROOT}" != "/" && "${REMOTE_ROOT}" != *".."* ]] \
        || fail "La racine distante enregistrée est invalide."
    [[ "${PUBLIC_URL:-}" =~ ^https://[A-Za-z0-9][A-Za-z0-9.-]*(:[0-9]+)?$ ]] \
        || fail "L’URL publique enregistrée est invalide."
    [[ "${ARCHITECTURES:-}" == "amd64" || "${ARCHITECTURES}" == "arm64" || "${ARCHITECTURES}" == "amd64,arm64" ]] \
        || fail "La liste d’architectures enregistrée est invalide."
}

load_configuration() {
    [[ -f "${CONFIG_FILE}" && ! -L "${CONFIG_FILE}" ]] \
        || fail "Configuration absente. Lancez d’abord : bash packaging/publish-release.sh configure"
    local mode owner
    mode="$(stat -c '%a' "${CONFIG_FILE}")"
    owner="$(stat -c '%u' "${CONFIG_FILE}")"
    [[ "${owner}" == "$(id -u)" && "${mode}" =~ ^(600|400)$ ]] \
        || fail "La configuration doit vous appartenir et être protégée en mode 0600."
    # Ce fichier est créé exclusivement par configure avec des valeurs échappées par printf %q.
    # shellcheck disable=SC1090
    source "${CONFIG_FILE}"
    validate_configuration
}

configure() {
    command -v gpg >/dev/null || fail "GnuPG est nécessaire pour choisir la clé de signature."
    printf '%s\n' "Clés privées disponibles :"
    gpg --list-secret-keys --with-subkey-fingerprint
    local signing_key remote_target remote_root public_url architectures temporary
    read -r -p "Empreinte complète de la clé principale : " signing_key
    read -r -p "Destination SSH (utilisateur@serveur) : " remote_target
    read -r -p "Racine distante [/var/www/packages.aegisadmin.fr] : " remote_root
    read -r -p "URL publique [https://packages.aegisadmin.fr] : " public_url
    read -r -p "Architectures [amd64] : " architectures
    remote_root="${remote_root:-/var/www/packages.aegisadmin.fr}"
    public_url="${public_url:-https://packages.aegisadmin.fr}"
    architectures="${architectures:-amd64}"
    SIGNING_KEY="${signing_key//[[:space:]]/}"
    REMOTE_TARGET="${remote_target}"
    REMOTE_ROOT="${remote_root}"
    PUBLIC_URL="${public_url%/}"
    ARCHITECTURES="${architectures}"
    validate_configuration
    gpg --batch --list-secret-keys "${SIGNING_KEY}" >/dev/null 2>&1 \
        || fail "La clé privée indiquée n’est pas disponible."
    install -d -m 0700 "$(dirname -- "${CONFIG_FILE}")"
    temporary="$(mktemp "$(dirname -- "${CONFIG_FILE}")/.publisher.conf.XXXXXXXX")"
    trap 'rm -f -- "${temporary}"' EXIT
    {
        printf 'SIGNING_KEY=%q\n' "${SIGNING_KEY}"
        printf 'REMOTE_TARGET=%q\n' "${REMOTE_TARGET}"
        printf 'REMOTE_ROOT=%q\n' "${REMOTE_ROOT}"
        printf 'PUBLIC_URL=%q\n' "${PUBLIC_URL}"
        printf 'ARCHITECTURES=%q\n' "${ARCHITECTURES}"
    } > "${temporary}"
    chmod 0600 "${temporary}"
    mv -f -- "${temporary}" "${CONFIG_FILE}"
    trap - EXIT
    printf 'Configuration enregistrée : %s\n' "${CONFIG_FILE}"
}

show_configuration() {
    load_configuration
    printf '%s\n' \
        "Empreinte GPG : ${SIGNING_KEY}" \
        "Destination SSH : ${REMOTE_TARGET}" \
        "Racine distante : ${REMOTE_ROOT}" \
        "URL publique : ${PUBLIC_URL}" \
        "Architectures : ${ARCHITECTURES}" \
        "Phrase secrète GPG : non enregistrée"
}

check_public_repository() {
    local inrelease_url="${PUBLIC_URL}/dists/stable/InRelease"
    if command -v curl >/dev/null; then
        curl --fail --silent --show-error "${inrelease_url}" >/dev/null
        return
    fi
    if command -v wget >/dev/null; then
        wget --quiet --output-document=/dev/null "${inrelease_url}"
        return
    fi
    fail "Le contrôle HTTPS final nécessite curl ou wget."
}

publish() {
    load_configuration
    for command in gpg rsync ssh stat; do
        command -v "${command}" >/dev/null || fail "Commande nécessaire absente : ${command}"
    done
    command -v curl >/dev/null || command -v wget >/dev/null \
        || fail "Le contrôle HTTPS final nécessite curl ou wget."
    local version release_directory remote_staging remote_command
    version="$(tr -d '[:space:]' < "${PROJECT_ROOT}/VERSION")"
    [[ "${version}" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.+~-][0-9A-Za-z.+:~-]+)?$ ]] \
        || fail "La version applicative est invalide : ${version}"
    release_directory="${PROJECT_ROOT}/dist/release-${version}"
    [[ ! -e "${release_directory}" ]] \
        || fail "La livraison existe déjà : ${release_directory}"
    gpg --batch --list-secret-keys "${SIGNING_KEY}" >/dev/null 2>&1 \
        || fail "La clé privée de signature n’est plus disponible."

    printf 'Préparation et signature de la version %s…\n' "${version}"
    bash "${SCRIPT_DIRECTORY}/build-release.sh" \
        --output "${release_directory}" \
        --signing-key "${SIGNING_KEY}" \
        --architectures "${ARCHITECTURES}" \
        --base-url "${PUBLIC_URL}"

    remote_staging="$(ssh "${REMOTE_TARGET}" \
        "mktemp -d /tmp/aegisadmin-publication-${version}.XXXXXXXX")"
    [[ "${remote_staging}" =~ ^/tmp/aegisadmin-publication-${version//./\.}\.[A-Za-z0-9]+$ ]] \
        || fail "Le serveur a retourné un répertoire temporaire invalide."
    printf 'Transfert vers %s:%s…\n' "${REMOTE_TARGET}" "${remote_staging}"
    rsync -a -- "${release_directory}/" "${REMOTE_TARGET}:${remote_staging}/release/"
    rsync -a -- "${SCRIPT_DIRECTORY}/" "${REMOTE_TARGET}:${remote_staging}/packaging/"

    printf -v remote_command 'cd %q && sudo bash packaging/publish-apt-repository.sh publish --repository release/repository --release %q --root %q && sudo bash packaging/publish-apt-repository.sh check --root %q' \
        "${remote_staging}" "${version}" "${REMOTE_ROOT}" "${REMOTE_ROOT}"
    printf 'Activation atomique de la version %s…\n' "${version}"
    ssh -t "${REMOTE_TARGET}" "${remote_command}"

    check_public_repository
    printf '%s\n' \
        "Publication ${version} terminée et contrôlée." \
        "Livraison locale : ${release_directory}" \
        "Copie de transfert conservée pour diagnostic : ${REMOTE_TARGET}:${remote_staging}"
}

case "${MODE}" in
    configure) configure ;;
    show-config) show_configuration ;;
    publish) publish ;;
esac
