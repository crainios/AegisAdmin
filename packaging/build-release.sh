#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'
umask 077

fail() { printf 'ERREUR : %s\n' "$1" >&2; exit "${2:-1}"; }
usage() {
    printf '%s\n' \
        "Usage : bash $0 --output REPERTOIRE --signing-key EMPREINTE [OPTIONS]" \
        "" \
        "Options :" \
        "  --architectures LISTE   Architectures séparées par des virgules (défaut : amd64)." \
        "  --base-url URL           URL APT (défaut : https://packages.aegisadmin.fr)." \
        "  --valid-days NOMBRE      Durée des métadonnées (défaut : 30)."
}

OUTPUT_DIRECTORY=""
SIGNING_KEY=""
ARCHITECTURES="amd64"
BASE_URL="https://packages.aegisadmin.fr"
VALID_DAYS=30

while (( $# > 0 )); do
    case "$1" in
        --output) (( $# >= 2 )) || fail "Répertoire de sortie absent." 2; OUTPUT_DIRECTORY="$2"; shift 2 ;;
        --signing-key) (( $# >= 2 )) || fail "Empreinte de signature absente." 2; SIGNING_KEY="$2"; shift 2 ;;
        --architectures) (( $# >= 2 )) || fail "Liste d’architectures absente." 2; ARCHITECTURES="$2"; shift 2 ;;
        --base-url) (( $# >= 2 )) || fail "URL publique absente." 2; BASE_URL="$2"; shift 2 ;;
        --valid-days) (( $# >= 2 )) || fail "Durée de validité absente." 2; VALID_DAYS="$2"; shift 2 ;;
        --help|-h) usage; exit 0 ;;
        *) usage >&2; fail "Option inconnue : $1" 2 ;;
    esac
done

[[ -n "${OUTPUT_DIRECTORY}" && ! -e "${OUTPUT_DIRECTORY}" ]] \
    || fail "Le répertoire de sortie doit être absent."
[[ "${SIGNING_KEY}" =~ ^[A-Fa-f0-9]{16,64}$ ]] || fail "L’empreinte GPG est invalide."

SCRIPT_DIRECTORY="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
OUTPUT_NAME="$(basename -- "${OUTPUT_DIRECTORY}")"
OUTPUT_PARENT="$(dirname -- "${OUTPUT_DIRECTORY}")"
if [[ ! -d "${OUTPUT_PARENT}" ]]; then
    install -d "${OUTPUT_PARENT}"
fi
OUTPUT_PARENT="$(cd -- "${OUTPUT_PARENT}" && pwd -P)"
OUTPUT_DIRECTORY="${OUTPUT_PARENT}/${OUTPUT_NAME}"
STAGING="$(mktemp -d "${OUTPUT_PARENT}/.aegisadmin-release.XXXXXXXX")"
trap 'rm -rf -- "${STAGING}"' EXIT
PACKAGES_DIRECTORY="${STAGING}/packages"
REPOSITORY_DIRECTORY="${STAGING}/repository"
install -d "${PACKAGES_DIRECTORY}"

IFS=',' read -r -a architecture_list <<< "${ARCHITECTURES}"
for architecture in "${architecture_list[@]}"; do
    case "${architecture}" in
        amd64|arm64) ;;
        *) fail "Architecture non prise en charge : ${architecture}" 2 ;;
    esac
    bash "${SCRIPT_DIRECTORY}/build-deb.sh" \
        --arch "${architecture}" \
        --output "${PACKAGES_DIRECTORY}"
done

bash "${SCRIPT_DIRECTORY}/build-apt-repository.sh" \
    --packages "${PACKAGES_DIRECTORY}" \
    --output "${REPOSITORY_DIRECTORY}" \
    --base-url "${BASE_URL}" \
    --signing-key "${SIGNING_KEY}" \
    --suite stable \
    --codename stable \
    --architectures "${ARCHITECTURES}" \
    --valid-days "${VALID_DAYS}"
bash "${SCRIPT_DIRECTORY}/check-apt-repository.sh" "${REPOSITORY_DIRECTORY}"

mv -- "${STAGING}" "${OUTPUT_DIRECTORY}"
trap - EXIT
printf 'Livraison AegisAdmin prête : %s\n' "${OUTPUT_DIRECTORY}"
