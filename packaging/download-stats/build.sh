#!/usr/bin/env bash

set -euo pipefail
IFS=$'\n\t'
umask 022

SCRIPT_DIRECTORY="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
PROJECT_ROOT="$(cd -- "${SCRIPT_DIRECTORY}/../.." && pwd -P)"
OUTPUT="${1:-${PROJECT_ROOT}/dist/aegisadmin-download-stats-amd64}"

command -v go >/dev/null || { printf 'ERROR: Go is required to build the statistics generator.\n' >&2; exit 1; }
mkdir -p -- "$(dirname -- "${OUTPUT}")"
(
    cd -- "${PROJECT_ROOT}/backend/go"
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w -buildid=' \
        -o "${OUTPUT}" ./cmd/aegisadmin-download-stats
)
chmod 0755 "${OUTPUT}"
printf 'Statistics generator built: %s\n' "${OUTPUT}"
