#!/usr/bin/env bash
# temp helper: append missing auth env vars to the remote nvenv (idempotent)
set -euo pipefail
cd "$(dirname "$0")"
source ./.deploy-env

: "${NV_SSH_HOST:=106.52.72.48}"
: "${NV_SSH_USER:=root}"
: "${NV_SSH_PORT:=22}"

JWT_SECRET="$(openssl rand -hex 32)"
ADMIN_PASSWORD="NvAdmin-$(openssl rand -hex 6)"

ASKPASS="$(mktemp)"
chmod 700 "$ASKPASS"
printf '#!/bin/sh\nprintf "%%s\\n" "$NV_SSH_PASSWORD"\n' > "$ASKPASS"
trap 'rm -f "$ASKPASS"' EXIT
export SSH_ASKPASS="$ASKPASS" SSH_ASKPASS_REQUIRE=force DISPLAY=:0

ssh -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -p "$NV_SSH_PORT" "$NV_SSH_USER@$NV_SSH_HOST" \
  "JWT_SECRET='$JWT_SECRET' ADMIN_PASSWORD='$ADMIN_PASSWORD' ENV_FILE='${NV_ENV_FILE:-nvenv}' bash -s" <<'EOF'
set -euo pipefail
ENV_PATH="/opt/new-vision/$ENV_FILE"
added=0
if ! grep -q '^NV_JWT_SECRET=' "$ENV_PATH"; then
  printf '\n# auth (added by deploy prep)\nNV_JWT_SECRET=%s\n' "$JWT_SECRET" >> "$ENV_PATH"
  added=1
  echo "NV_JWT_SECRET: appended"
else
  echo "NV_JWT_SECRET: already present"
fi
if ! grep -q '^NV_SEED_ADMIN_PASSWORD=' "$ENV_PATH"; then
  printf 'NV_SEED_ADMIN_PASSWORD=%s\n' "$ADMIN_PASSWORD" >> "$ENV_PATH"
  echo "NV_SEED_ADMIN_PASSWORD: appended"
else
  echo "NV_SEED_ADMIN_PASSWORD: already present"
fi
[ "$added" = "1" ] && chmod 600 "$ENV_PATH"
echo "env file now has $(grep -cE '^[A-Z_]+' "$ENV_PATH") vars"
EOF

echo "ADMIN_PASSWORD=$ADMIN_PASSWORD"
