#!/usr/bin/env bash
# Starts Grafana and the PI Web API simulator (every time the codespace starts).
# GRAFANA_VERSION=11.6.0 .devcontainer/start.sh switches the Grafana version.
set -euo pipefail
cd "$(dirname "$0")"

if [ -n "${CODESPACE_NAME:-}" ]; then
  export GRAFANA_HOST="${CODESPACE_NAME}-3000.${GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN}"
  export GRAFANA_ORIGIN="https://${GRAFANA_HOST}"
  export GRAFANA_URL="${GRAFANA_ORIGIN}/"
fi

for i in $(seq 60); do docker info >/dev/null 2>&1 && break; sleep 1; done
docker compose up -d --pull missing --force-recreate

for i in $(seq 90); do curl -sf localhost:3000/api/health >/dev/null && break; sleep 2; done
echo "Grafana ${GRAFANA_VERSION:-13.2.2}: ${GRAFANA_URL:-http://localhost:3000/} (dashboards in the \"PI Web API simulator\" folder)"
