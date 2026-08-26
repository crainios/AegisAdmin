#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'
umask 077

fail() { printf 'ERREUR : %s\n' "$1" >&2; exit "${2:-1}"; }
usage() {
    printf '%s\n' \
        "Usage : bash $0 --packages REPERTOIRE --output REPERTOIRE --base-url URL --signing-key EMPREINTE [OPTIONS]" \
        "" \
        "Options :" \
        "  --suite NOM             Suite APT (défaut : stable)." \
        "  --codename NOM          Nom de code (défaut : valeur de --suite)." \
        "  --architectures LISTE   Architectures séparées par des virgules (défaut : amd64)." \
        "  --valid-days NOMBRE     Durée de validité des métadonnées (défaut : 30)."
}

PACKAGES_DIRECTORY=""
OUTPUT_DIRECTORY=""
BASE_URL=""
SIGNING_KEY=""
SUITE="stable"
CODENAME=""
ARCHITECTURES="amd64"
VALID_DAYS=30

while (( $# > 0 )); do
    case "$1" in
        --packages) (( $# >= 2 )) || fail "Répertoire de paquets absent." 2; PACKAGES_DIRECTORY="$2"; shift 2 ;;
        --output) (( $# >= 2 )) || fail "Répertoire de sortie absent." 2; OUTPUT_DIRECTORY="$2"; shift 2 ;;
        --base-url) (( $# >= 2 )) || fail "URL publique absente." 2; BASE_URL="$2"; shift 2 ;;
        --signing-key) (( $# >= 2 )) || fail "Empreinte de signature absente." 2; SIGNING_KEY="$2"; shift 2 ;;
        --suite) (( $# >= 2 )) || fail "Suite absente." 2; SUITE="$2"; shift 2 ;;
        --codename) (( $# >= 2 )) || fail "Nom de code absent." 2; CODENAME="$2"; shift 2 ;;
        --architectures) (( $# >= 2 )) || fail "Liste d’architectures absente." 2; ARCHITECTURES="$2"; shift 2 ;;
        --valid-days) (( $# >= 2 )) || fail "Durée de validité absente." 2; VALID_DAYS="$2"; shift 2 ;;
        --help|-h) usage; exit 0 ;;
        *) usage >&2; fail "Option inconnue : $1" 2 ;;
    esac
done

[[ -d "${PACKAGES_DIRECTORY}" && ! -L "${PACKAGES_DIRECTORY}" ]] || fail "Répertoire de paquets invalide."
[[ -n "${OUTPUT_DIRECTORY}" && ! -e "${OUTPUT_DIRECTORY}" ]] || fail "Le répertoire de sortie doit être absent."
[[ "${BASE_URL}" =~ ^https://[^[:space:]]+/?$ ]] || fail "L’URL publique doit utiliser HTTPS."
[[ "${SIGNING_KEY}" =~ ^[A-Fa-f0-9]{16,64}$ ]] || fail "L’empreinte GPG est invalide."
[[ "${SUITE}" =~ ^[a-z0-9][a-z0-9.-]*$ ]] || fail "La suite APT est invalide."
[[ "${VALID_DAYS}" =~ ^[0-9]+$ && "${VALID_DAYS}" -ge 1 && "${VALID_DAYS}" -le 365 ]] || fail "La durée de validité doit être comprise entre 1 et 365 jours."
CODENAME="${CODENAME:-${SUITE}}"
[[ "${CODENAME}" =~ ^[a-z0-9][a-z0-9.-]*$ ]] || fail "Le nom de code est invalide."

for command in apt-ftparchive cat chmod date dpkg-deb dpkg-scanpackages find gpg gzip install mktemp mv rm sha256sum xz; do
    command -v "${command}" >/dev/null || fail "Commande de publication absente : ${command}" 3
done
gpg --batch --list-secret-keys "${SIGNING_KEY}" >/dev/null 2>&1 \
    || fail "La clé privée de signature n’est pas disponible dans le trousseau GPG."

mapfile -t packages < <(find "${PACKAGES_DIRECTORY}" -maxdepth 1 -type f -name 'aegisadmin_*.deb' -print | sort)
(( ${#packages[@]} > 0 )) || fail "Aucun paquet AegisAdmin n’a été trouvé."

parent="$(dirname -- "${OUTPUT_DIRECTORY}")"
if [[ ! -d "${parent}" ]]; then
    install -d "${parent}"
fi
staging="$(mktemp -d "${parent}/.aegisadmin-apt.XXXXXXXX")"
trap 'rm -rf -- "${staging}"' EXIT
pool="${staging}/pool/main/a/aegisadmin"
install -d "${pool}"

for package in "${packages[@]}"; do
    [[ "$(dpkg-deb -f "${package}" Package)" == aegisadmin ]] || fail "Paquet étranger refusé : ${package}"
    bash "$(dirname -- "${BASH_SOURCE[0]}")/check-deb.sh" "${package}"
    install -m 0644 "${package}" "${pool}/$(basename -- "${package}")"
done

IFS=',' read -r -a architecture_list <<< "${ARCHITECTURES}"
for architecture in "${architecture_list[@]}"; do
    [[ "${architecture}" =~ ^[a-z0-9][a-z0-9-]*$ ]] || fail "Architecture invalide : ${architecture}"
    binary_directory="${staging}/dists/${SUITE}/main/binary-${architecture}"
    install -d "${binary_directory}"
    (
        cd -- "${staging}"
        dpkg-scanpackages --arch "${architecture}" pool > "${binary_directory}/Packages"
    )
    gzip -n -9 -c "${binary_directory}/Packages" > "${binary_directory}/Packages.gz"
    xz -9e -c "${binary_directory}/Packages" > "${binary_directory}/Packages.xz"
    install -d "${binary_directory}/by-hash/SHA256"
    for index in Packages Packages.gz Packages.xz; do
        digest="$(sha256sum "${binary_directory}/${index}")"
        digest="${digest%% *}"
        install -m 0644 "${binary_directory}/${index}" "${binary_directory}/by-hash/SHA256/${digest}"
    done
done

release_directory="${staging}/dists/${SUITE}"
valid_until="$(date --utc --date="+${VALID_DAYS} days" --rfc-email)"
generated_release="${staging}/Release.generated"
(
    cd -- "${staging}"
    apt-ftparchive \
        -o "APT::FTPArchive::Release::Origin=AegisAdmin" \
        -o "APT::FTPArchive::Release::Label=AegisAdmin" \
        -o "APT::FTPArchive::Release::Suite=${SUITE}" \
        -o "APT::FTPArchive::Release::Codename=${CODENAME}" \
        -o "APT::FTPArchive::Release::Architectures=${ARCHITECTURES//,/ }" \
        -o "APT::FTPArchive::Release::Components=main" \
        -o "APT::FTPArchive::Release::Description=AegisAdmin signed package repository" \
        -o "APT::FTPArchive::Release::Acquire-By-Hash=yes" \
        release "dists/${SUITE}" > "${generated_release}"
)
{
    printf 'Valid-Until: %s\n' "${valid_until}"
    cat "${generated_release}"
} > "${release_directory}/Release"
rm -f -- "${generated_release}"

gpg --batch --yes --local-user "${SIGNING_KEY}" --digest-algo SHA256 \
    --clearsign --output "${release_directory}/InRelease" "${release_directory}/Release"
gpg --batch --yes --local-user "${SIGNING_KEY}" --digest-algo SHA256 \
    --armor --detach-sign --output "${release_directory}/Release.gpg" "${release_directory}/Release"
gpg --batch --yes --export "${SIGNING_KEY}" > "${staging}/aegisadmin-archive-keyring.gpg"
gpg --batch --yes --armor --export "${SIGNING_KEY}" > "${staging}/aegisadmin-archive-keyring.asc"

base_url="${BASE_URL%/}"
cat > "${staging}/aegisadmin.sources" <<EOF
Types: deb
URIs: ${base_url}
Suites: ${SUITE}
Components: main
Architectures: ${ARCHITECTURES//,/ }
Signed-By: /usr/share/keyrings/aegisadmin-archive-keyring.gpg
EOF

find "${staging}" -type d -exec chmod 0755 {} +
find "${staging}" -type f -exec chmod 0644 {} +

mv -- "${staging}" "${OUTPUT_DIRECTORY}"
trap - EXIT
printf 'Dépôt APT signé créé : %s\n' "${OUTPUT_DIRECTORY}"
