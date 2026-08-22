#!/usr/bin/env bash
# temp helper: inspect remote env + service state
set -euo pipefail
cd "$(dirname "$0")"
source ./.deploy-env

ASKPASS="$(mktemp)"
chmod 700 "$ASKPASS"
printf '#!/bin/sh\nprintf "%%s\\n" "$NV_SSH_PASSWORD"\n' > "$ASKPASS"
trap 'rm -f "$ASKPASS"' EXIT
export SSH_ASKPASS="$ASKPASS" SSH_ASKPASS_REQUIRE=force DISPLAY=:0

ssh -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -p "${NV_SSH_PORT:-22}" "${NV_SSH_USER:-root}@${NV_SSH_HOST:-106.52.72.48}" 'bash -s' <<'EOF'
echo "--- nvenv keys ---"
if [ -f /opt/new-vision/nvenv ]; then
  grep -oE '^[A-Z_]+' /opt/new-vision/nvenv
else
  echo "nvenv NOT FOUND"
fi
echo "--- compose services ---"
if [ -d /opt/new-vision ]; then
  cd /opt/new-vision
  docker compose --env-file nvenv ps --format '{{.Service}} {{.State}} {{.Health}}' 2>/dev/null | head -12
  echo "commit: $(git rev-parse --short HEAD 2>/dev/null) branch: $(git rev-parse --abbrev-ref HEAD 2>/dev/null)"
else
  echo "no deploy dir"
fi
EOF
