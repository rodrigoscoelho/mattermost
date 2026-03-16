#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVER_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_DIR="$(cd "${SERVER_DIR}/.." && pwd)"
WEBAPP_DIST_DIR="${REPO_DIR}/webapp/channels/dist"
REMOTE="${REMOTE:-root@192.168.1.2}"
REMOTE_DIR="${REMOTE_DIR:-/opt/mattermost.new}"
RSYNC_RSH="${RSYNC_RSH:-ssh}"
STAGE_DIR="$(mktemp -d)"
DRY_RUN=false

usage() {
    cat <<EOF
Uso:
  $(basename "$0")
  $(basename "$0") --dry-run

Variaveis opcionais:
  REMOTE=root@192.168.1.2
  REMOTE_DIR=/opt/mattermost.new
  RSYNC_RSH="ssh -i /caminho/da/chave"

Este script sincroniza apenas o que sai do build:
  - bin/mattermost
  - bin/mmctl (se existir)
  - client/ (webapp/channels/dist)
  - fonts/
  - i18n/
  - templates/
  - NOTICE.txt
  - README.md
  - MIT-COMPILED-LICENSE.md (se existir)
  - ENTERPRISE-EDITION-LICENSE.txt (se existir)

Ele nao toca em:
  - config/
  - data/
  - logs/
  - plugins/
  - prepackaged_plugins/
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    usage
    exit 0
fi

if [[ "${1:-}" == "--dry-run" ]]; then
    DRY_RUN=true
fi

require_path() {
    local path="$1"
    if [[ ! -e "$path" ]]; then
        echo "artefato ausente: $path" >&2
        echo "rode 'make build-cmd' em ${SERVER_DIR} antes de sincronizar" >&2
        exit 1
    fi
}

stage_dir() {
    local src="$1"
    local name="$2"
    require_path "$src"
    mkdir -p "${STAGE_DIR}/${name}"
    rsync -a --delete "${src}/" "${STAGE_DIR}/${name}/"
}

stage_file() {
    local src="$1"
    local dest_name="$2"
    if [[ -e "$src" ]]; then
        cp -a "$src" "${STAGE_DIR}/${dest_name}"
    fi
}

cleanup() {
    rm -rf "${STAGE_DIR}"
}

require_path "${SERVER_DIR}/bin/mattermost"
require_path "${WEBAPP_DIST_DIR}"
require_path "${SERVER_DIR}/fonts"
require_path "${SERVER_DIR}/i18n"
require_path "${SERVER_DIR}/templates"
require_path "${REPO_DIR}/NOTICE.txt"
require_path "${REPO_DIR}/README.md"

echo "sincronizando build local para ${REMOTE}:${REMOTE_DIR}"
if [[ "${DRY_RUN}" == "true" ]]; then
    echo "modo dry-run ativo"
fi

trap cleanup EXIT

mkdir -p "${STAGE_DIR}/bin"
cp -a "${SERVER_DIR}/bin/mattermost" "${STAGE_DIR}/bin/"
if [[ -e "${SERVER_DIR}/bin/mmctl" ]]; then
    cp -a "${SERVER_DIR}/bin/mmctl" "${STAGE_DIR}/bin/"
fi

stage_dir "${WEBAPP_DIST_DIR}" "client"
stage_dir "${SERVER_DIR}/fonts" "fonts"
stage_dir "${SERVER_DIR}/i18n" "i18n"
stage_dir "${SERVER_DIR}/templates" "templates"
stage_file "${REPO_DIR}/NOTICE.txt" "NOTICE.txt"
stage_file "${REPO_DIR}/README.md" "README.md"
stage_file "${SERVER_DIR}/build/MIT-COMPILED-LICENSE.md" "MIT-COMPILED-LICENSE.md"
stage_file "${REPO_DIR}/enterprise/ENTERPRISE-EDITION-LICENSE.txt" "ENTERPRISE-EDITION-LICENSE.txt"
stage_file "${SERVER_DIR}/bin/manifest.txt" "manifest.txt"

sources=(
    "${STAGE_DIR}/bin"
    "${STAGE_DIR}/client"
    "${STAGE_DIR}/fonts"
    "${STAGE_DIR}/i18n"
    "${STAGE_DIR}/templates"
    "${STAGE_DIR}/NOTICE.txt"
    "${STAGE_DIR}/README.md"
)

for optional_path in \
    "${STAGE_DIR}/MIT-COMPILED-LICENSE.md" \
    "${STAGE_DIR}/ENTERPRISE-EDITION-LICENSE.txt" \
    "${STAGE_DIR}/manifest.txt"
do
    if [[ -e "${optional_path}" ]]; then
        sources+=("${optional_path}")
    fi
done

rsync_args=(
    -azh
    --delete
    --progress
    -e "${RSYNC_RSH}"
)

if [[ "${DRY_RUN}" == "true" ]]; then
    rsync_args+=(--dry-run --itemize-changes)
fi

rsync "${rsync_args[@]}" "${sources[@]}" "${REMOTE}:${REMOTE_DIR}/"

echo "sincronizacao concluida"
