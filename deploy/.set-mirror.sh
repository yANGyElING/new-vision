#!/usr/bin/env bash
# temp helper: rewrite github.com fetches through gh-proxy.com on the server
set -euo pipefail
cd "$(dirname "$0")"
source ./.deploy-env

: "${NV_SSH_HOST:=106.52.72.48}"
: "${NV_SSH_USER:=root}"
: "${NV_SSH_PORT:=22}"

ASKPASS="$(mktemp)"
chmod 700 "$ASKPASS"
printf '#!/bin/sh\nprintf "%%s\\n" "$NV_SSH_PASSWORD"\n' > "$ASKPASS"
trap 'rm -f "$ASKPASS"' EXIT
export SSH_ASKPASS="$ASKPASS" SSH_ASKPASS_REQUIRE=force DISPLAY=:0

ssh -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -p "$NV_SSH_PORT" "$NV_SSH_USER@$NV_SSH_HOST" 'bash -s' <<'EOF'
set -euo pipefail
git config --global url."https://gh-proxy.com/https://github.com/".insteadOf "https://github.com/"
echo "insteadOf rule:"
git config --global --get-regexp 'url\..*\.insteadof'
# smoke test: fetch through the mirror
cd /opt/new-vision
git fetch origin main 2>&1 | tail -2 || true
echo "fetch exit: $?"
EOF
