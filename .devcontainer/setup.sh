#!/usr/bin/env bash
# Runs once when the codespace is created: installs the dependencies and builds the plugin.
set -euo pipefail
cd "$(dirname "$0")/.."
# Corepack would otherwise wait for a confirmation before downloading yarn.
export COREPACK_ENABLE_DOWNLOAD_PROMPT=0

command -v yarn >/dev/null || npm install -g yarn
command -v mage >/dev/null || go install github.com/magefile/mage@latest

yarn install --frozen-lockfile
.devcontainer/build.sh
