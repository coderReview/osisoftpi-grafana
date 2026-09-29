#!/usr/bin/env bash
# Builds the frontend and the Linux backend into dist/ and restarts Grafana if it is running.
set -euo pipefail
cd "$(dirname "$0")/.."
export PATH="$PATH:$(go env GOPATH)/bin"

yarn build
mage -v build:linux

if docker ps --format '{{.Names}}' | grep -qx grafana; then
  docker restart grafana >/dev/null
  echo "Grafana restarted with the new build."
fi
