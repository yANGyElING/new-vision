#!/usr/bin/env bash
# temp helper: probe which GitHub access channels work from the server
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
probe() {
  local name="$1" url="$2"
  if curl -fsS -m 8 -o /dev/null "$url" 2>/dev/null; then
    echo "OK    $name"
  else
    echo "FAIL  $name"
  fi
}
probe "github.com direct"      "https://github.com/yANGyElING/new-vision.git/info/refs?service=git-upload-pack"
probe "ssh.github.com:443"     "https://ssh.github.com"
probe "gh-proxy.com"           "https://gh-proxy.com/https://github.com/yANGyElING/new-vision.git/info/refs?service=git-upload-pack"
probe "ghfast.top"             "https://ghfast.top/https://github.com/yANGyElING/new-vision.git/info/refs?service=git-upload-pack"
probe "ghproxy.net"            "https://ghproxy.net/https://github.com/yANGyElING/new-vision.git/info/refs?service=git-upload-pack"
probe "gitclone.com"           "https://gitclone.com/github.com/yANGyElING/new-vision.git/info/refs?service=git-upload-pack"
probe "hub.gitmirror.com"      "https://hub.gitmirror.com/https://github.com/yANGyElING/new-vision.git/info/refs?service=git-upload-pack"
echo "--- existing proxy config ---"
git config --global --get http.proxy || echo "no git http.proxy"
env | grep -i proxy || echo "no proxy env"
EOF
