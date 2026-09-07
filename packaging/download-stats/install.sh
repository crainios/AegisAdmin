#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'
umask 027

fail() { printf 'ERROR: %s\n' "$1" >&2; exit 1; }
(( EUID == 0 )) || fail "Run this installer with sudo."
(( $# == 1 )) || fail "Usage: sudo bash install.sh /path/to/aegisadmin-download-stats-amd64"

BINARY="$1"
SCRIPT_DIRECTORY="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
[[ -f "${BINARY}" && ! -L "${BINARY}" ]] || fail "The compiled generator is missing."
[[ -s /etc/apache2/aegisadmin-download-stats.htpasswd ]] \
    || fail "Create /etc/apache2/aegisadmin-download-stats.htpasswd with htpasswd before installation."
command -v apache2ctl >/dev/null || fail "Apache is required on the package repository server."
command -v a2enconf >/dev/null || fail "a2enconf is unavailable."

install -d -o root -g root -m 0755 /usr/local/libexec
install -o root -g root -m 0755 "${BINARY}" /usr/local/libexec/aegisadmin-download-stats
install -o root -g root -m 0644 "${SCRIPT_DIRECTORY}/aegisadmin-download-stats.service" /etc/systemd/system/aegisadmin-download-stats.service
install -o root -g root -m 0644 "${SCRIPT_DIRECTORY}/aegisadmin-download-stats.timer" /etc/systemd/system/aegisadmin-download-stats.timer
install -o root -g root -m 0644 "${SCRIPT_DIRECTORY}/apache.conf" /etc/apache2/conf-available/aegisadmin-download-stats.conf

systemctl daemon-reload
a2enconf aegisadmin-download-stats >/dev/null
if ! apache2ctl configtest; then
    a2disconf aegisadmin-download-stats >/dev/null || true
    fail "Apache rejected the dashboard configuration; it has been disabled."
fi
systemctl reload apache2
systemctl enable --now aegisadmin-download-stats.timer
systemctl start aegisadmin-download-stats.service
printf 'Dashboard installed at https://packages.aegisadmin.fr/private-downloads/\n'
