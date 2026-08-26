#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'

fail() { printf 'ERREUR : %s\n' "$1" >&2; exit "${2:-1}"; }
(( $# == 1 )) || fail "Usage : bash $0 REPERTOIRE_DU_DEPOT" 2
readonly REPOSITORY="$1"
[[ -d "${REPOSITORY}" && ! -L "${REPOSITORY}" ]] || fail "Répertoire de dépôt invalide."

for command in awk find gpgv sha256sum stat; do
    command -v "${command}" >/dev/null || fail "Commande de contrôle absente : ${command}" 3
done
[[ -z "$(find "${REPOSITORY}" -type l -print -quit)" ]] \
    || fail "Le dépôt ne doit contenir aucun lien symbolique."
readonly KEYRING="${REPOSITORY}/aegisadmin-archive-keyring.gpg"
[[ -s "${KEYRING}" ]] || fail "Clé publique absente."

mapfile -t releases < <(find "${REPOSITORY}/dists" -mindepth 2 -maxdepth 2 -type f -name Release -print | sort)
(( ${#releases[@]} > 0 )) || fail "Aucun fichier Release trouvé."
for release in "${releases[@]}"; do
    directory="$(dirname -- "${release}")"
    [[ -s "${directory}/InRelease" && -s "${directory}/Release.gpg" ]] || fail "Signatures absentes dans ${directory}."
    gpgv --keyring "${KEYRING}" "${directory}/InRelease" >/dev/null
    gpgv --keyring "${KEYRING}" "${directory}/Release.gpg" "${release}" >/dev/null
    grep -q '^Acquire-By-Hash: yes$' "${release}" || fail "Acquire-By-Hash absent de ${release}."
    grep -q '^Valid-Until: ' "${release}" || fail "Valid-Until absent de ${release}."

    while IFS=$'\t' read -r expected_hash expected_size relative_path; do
        [[ -n "${relative_path}" ]] || continue
        [[ "${relative_path}" != /* && "${relative_path}" != *..* ]] \
            || fail "Chemin dangereux dans ${release} : ${relative_path}"
        indexed_file="${directory}/${relative_path}"
        [[ -f "${indexed_file}" && ! -L "${indexed_file}" ]] \
            || fail "Fichier indexé absent : ${indexed_file}"
        [[ "$(stat -c '%s' "${indexed_file}")" == "${expected_size}" ]] \
            || fail "Taille incorrecte : ${indexed_file}"
        actual_hash="$(sha256sum "${indexed_file}")"
        actual_hash="${actual_hash%% *}"
        [[ "${actual_hash}" == "${expected_hash}" ]] \
            || fail "Somme SHA-256 incorrecte : ${indexed_file}"
    done < <(awk '
        /^SHA256:$/ { section=1; next }
        section && /^[A-Za-z][A-Za-z0-9-]*:/ { exit }
        section && NF == 3 { print $1 "\t" $2 "\t" $3 }
    ' "${release}")
done

mapfile -t package_indexes < <(find "${REPOSITORY}/dists" -type f -name Packages -print | sort)
(( ${#package_indexes[@]} > 0 )) || fail "Aucun index Packages trouvé."
for package_index in "${package_indexes[@]}"; do
    grep -q '^Package: aegisadmin$' "${package_index}" || fail "Index sans paquet AegisAdmin : ${package_index}"
    grep -q '^SHA256: ' "${package_index}" || fail "Somme SHA-256 absente de ${package_index}"

    while IFS=$'\t' read -r relative_path expected_size expected_hash; do
        [[ -n "${relative_path}" ]] || continue
        [[ "${relative_path}" != /* && "${relative_path}" != *..* ]] \
            || fail "Chemin de paquet dangereux : ${relative_path}"
        package_file="${REPOSITORY}/${relative_path}"
        [[ -f "${package_file}" && ! -L "${package_file}" ]] \
            || fail "Paquet indexé absent : ${package_file}"
        [[ "$(stat -c '%s' "${package_file}")" == "${expected_size}" ]] \
            || fail "Taille de paquet incorrecte : ${package_file}"
        actual_hash="$(sha256sum "${package_file}")"
        actual_hash="${actual_hash%% *}"
        [[ "${actual_hash}" == "${expected_hash}" ]] \
            || fail "Somme SHA-256 du paquet incorrecte : ${package_file}"
    done < <(awk '
        BEGIN { RS=""; FS="\n" }
        {
            filename=size=hash=""
            for (i=1; i<=NF; i++) {
                if ($i ~ /^Filename: /) { filename=substr($i, 11) }
                if ($i ~ /^Size: /) { size=substr($i, 7) }
                if ($i ~ /^SHA256: /) { hash=substr($i, 9) }
            }
            if (filename != "" && size != "" && hash != "") {
                print filename "\t" size "\t" hash
            }
        }
    ' "${package_index}")
done

printf 'Dépôt APT AegisAdmin vérifié : %s\n' "${REPOSITORY}"
