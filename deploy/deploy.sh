#!/usr/bin/env bash
# one-click deploy: git update on remote -> build -> deploy -> migrate -> verify, with auto-rollback.
#
# run on the DEV machine (Git Bash on Windows, or Linux/macOS bash).
#
# usage:
#   export NV_SSH_PASSWORD='your-ssh-password'
#   export NV_GIT_REPO='https://github.com/your-org/new-vision.git'
#   bash deploy/deploy.sh
#
# flow: SSH to server -> (clone | pull) -> generate/preserve env -> validate compose
#       -> snapshot previous images -> docker compose build -> up -d -> wait healthy
#       -> migrate -> health check.
# on any failure after a successful pull, rolls back: git reset to previous commit,
# retag :prev images back, restart the old stack.
#
# configurable via env (defaults match the current test server):
#   NV_SSH_HOST      SSH host (default 106.52.72.48)
#   NV_SSH_PORT      SSH port (default 22)
#   NV_SSH_USER      SSH user (default root)
#   NV_SSH_PASSWORD  SSH password (required, never stored in this file)
#   NV_GIT_REPO      git repository URL (required; public HTTPS needs no credentials)
#   NV_GIT_BRANCH    branch to deploy (default master)
#   NV_REMOTE_DIR    deploy dir on server (default /opt/new-vision)
#   NV_ENV_FILE      env file name (default nvenv; generated on first install, preserved across updates)
#   NV_GOPROXY       go proxy for node-app build (default goproxy.cn for CN networks)
#   NV_TIMEOUT       health wait timeout in seconds (default 180)
#
# server prerequisites (detected, not installed): docker + compose v2 + git + curl
# (openssl only needed to generate the first env file).

set -euo pipefail

SSH_HOST="${NV_SSH_HOST:-106.52.72.48}"
SSH_PORT="${NV_SSH_PORT:-22}"
SSH_USER="${NV_SSH_USER:-root}"
: "${NV_SSH_PASSWORD:?NV_SSH_PASSWORD not set - export it first}"
: "${NV_GIT_REPO:?NV_GIT_REPO not set - export the repository URL first}"
GIT_BRANCH="${NV_GIT_BRANCH:-master}"
REMOTE_DIR="${NV_REMOTE_DIR:-/opt/new-vision}"
ENV_FILE="${NV_ENV_FILE:-nvenv}"
GOPROXY="${NV_GOPROXY:-https://goproxy.cn,direct}"
TIMEOUT="${NV_TIMEOUT:-180}"

export NV_SSH_PASSWORD

# password auth requires OpenSSH >= 8.4 (SSH_ASKPASS_REQUIRE=force)
SSH_VER="$(ssh -V 2>&1 | grep -oE 'OpenSSH_[0-9.]+' | head -1 || true)"
case "$SSH_VER" in
  OpenSSH_8.[4-9]*|OpenSSH_9.*|OpenSSH_1[0-9].*) ;;
  *) echo "ERROR: OpenSSH >= 8.4 required for password auth (found ${SSH_VER:-unknown}); upgrade your SSH client or use a newer Git Bash." >&2; exit 1 ;;
esac

# non-interactive password auth via ssh-askpass (OpenSSH >= 8.4)
ASKPASS="$(mktemp)"
chmod 700 "$ASKPASS"
printf '#!/bin/sh\nprintf "%%s\\n" "$NV_SSH_PASSWORD"\n' > "$ASKPASS"
trap 'rm -f "$ASKPASS"' EXIT
export SSH_ASKPASS="$ASKPASS"
export SSH_ASKPASS_REQUIRE=force

ssh_run() {
  ssh -o ConnectTimeout=15 -o StrictHostKeyChecking=accept-new -p "$SSH_PORT" "$SSH_USER@$SSH_HOST" "$@"
}

echo "== deploy $GIT_BRANCH -> $SSH_USER@$SSH_HOST:$REMOTE_DIR =="
echo "   (one SSH connection; node-access rebuilds Kamailio, this can take a while)"

if ssh_run "REMOTE_DIR='$REMOTE_DIR' GIT_REPO='$GIT_REPO' GIT_BRANCH='$GIT_BRANCH' ENV_FILE='$ENV_FILE' GOPROXY='$GOPROXY' TIMEOUT='$TIMEOUT' bash -s" <<'REMOTE_EOF'
set -euo pipefail

log() { printf '\n\033[1;36m== %s ==\033[0m\n' "$*"; }
die() { printf '\033[1;31mERROR: %s\033[0m\n' "$*" >&2; rollback; exit 1; }

ROLLBACK_DONE=0
PREV_COMMIT=""

rollback() {
  [ "$ROLLBACK_DONE" = "1" ] && return 0
  ROLLBACK_DONE=1
  set +e
  echo
  echo "!! DEPLOY FAILED - rolling back"
  if [ -n "$PREV_COMMIT" ]; then
    cd "$REMOTE_DIR" 2>/dev/null || return 0
    git reset --hard "$PREV_COMMIT" >/dev/null 2>&1
    echo "  source reset to $(git rev-parse --short HEAD 2>/dev/null)"
    docker images --format '{{.Repository}}:{{.Tag}}' | awk -F: '/^new-vision\/.*:prev$/ {print $1" "$2}' | while read -r repo tag; do
      docker tag "$repo:$tag" "$repo" >/dev/null 2>&1
    done
    docker compose --env-file "$ENV_FILE" up -d >/dev/null 2>&1
    echo "  old stack restarted (best effort)"
  else
    echo "  no previous version recorded (first install) - nothing to restore"
  fi
  echo "!! rollback finished - inspect the server if services are still down"
  set -e
}
trap 'rollback' ERR

# --- [1/7] preflight ---------------------------------------------------------
log "1/7 remote preflight"
command -v docker >/dev/null 2>&1 || die "docker not found on server - install Docker Engine first"
docker compose version >/dev/null 2>&1 || die "docker compose v2 not found on server - install it first"
command -v git >/dev/null 2>&1 || die "git not found on server"
command -v curl >/dev/null 2>&1 || die "curl not found on server"
echo "  docker: $(docker --version)"
echo "  compose: $(docker compose version --short)"

# --- [2/7] source update -----------------------------------------------------
log "2/7 source update"
if [ -d "$REMOTE_DIR/.git" ]; then
  cd "$REMOTE_DIR"
  PREV_COMMIT="$(git rev-parse HEAD)"
  git fetch origin
  git checkout "$GIT_BRANCH"
  git pull --ff-only origin "$GIT_BRANCH"
  echo "  updated: $(echo "$PREV_COMMIT" | cut -c1-7) -> $(git rev-parse --short HEAD) ($GIT_BRANCH)"
elif [ -d "$REMOTE_DIR" ] && [ -f "$REMOTE_DIR/compose.yaml" ]; then
  die "$REMOTE_DIR exists but is not a git clone; refusing to overwrite (deploy via git only)"
else
  git clone --branch "$GIT_BRANCH" "$GIT_REPO" "$REMOTE_DIR"
  cd "$REMOTE_DIR"
  echo "  cloned: $(git rev-parse --short HEAD) ($GIT_BRANCH)"
fi

# --- [3/7] env file ----------------------------------------------------------
log "3/7 env file"
ENV_PATH="$REMOTE_DIR/$ENV_FILE"
if [ -f "$ENV_PATH" ]; then
  echo "  preserving existing env file: $ENV_PATH"
else
  command -v openssl >/dev/null 2>&1 || die "openssl not found on server (needed to generate secrets)"
  echo "  generating $ENV_PATH with random passwords"
  PG_PW="$(openssl rand -hex 16)"
  RD_PW="$(openssl rand -hex 16)"
  RD_NODEAPP_PW="$(openssl rand -hex 16)"
  RD_ACCESS_PW="$(openssl rand -hex 16)"
  cat > "$ENV_PATH" <<EOF
NV_HTTP_PORT=8080
NV_HTTP_ADDR=:8080
NV_SIP_PORT=5060
NV_LOG_LEVEL=info
NV_SHUTDOWN_TIMEOUT=10s
NV_HEALTH_TIMEOUT=1s

NV_POSTGRES_DB=new_vision
NV_POSTGRES_USER=new_vision
NV_POSTGRES_PASSWORD=$PG_PW
NV_POSTGRES_SSLMODE=disable

NV_REDIS_PASSWORD=$RD_PW
NV_REDIS_NODEAPP_PASSWORD=$RD_NODEAPP_PW
NV_REDIS_ACCESS_PASSWORD=$RD_ACCESS_PW
NV_REDIS_DB=0

NV_ACCESS_RPC_URL=http://node-access:8090/rpc
NV_ACCESS_RPC_TIMEOUT=3s
NV_ACCESS_POLL_INTERVAL=1s
NV_ACCESS_INSTANCE_ID=access-01
NV_KEEPALIVE_TIMEOUT=180
NV_SIP_REALM=3402000000
EOF
  chmod 600 "$ENV_PATH"
  echo "  passwords are in $ENV_PATH - keep it safe"
fi

# --- [4/7] validate + snapshot previous images ------------------------------
log "4/7 validate compose + snapshot previous images"
cd "$REMOTE_DIR"
docker compose --env-file "$ENV_FILE" config --quiet || die "compose config invalid"
if [ -n "$PREV_COMMIT" ]; then
  docker images --format '{{.Repository}}:{{.Tag}}' | awk -F: '/^new-vision\// && $2 != "prev" {print}' | while read -r img; do
    docker tag "$img" "${img%%:*}:prev" >/dev/null 2>&1 || true
  done
  echo "  previous new-vision images tagged :prev"
fi

# --- [5/7] build -------------------------------------------------------------
log "5/7 build (node-access compiles Kamailio 6.1.3 - this can take a while)"
export NV_GOPROXY="$GOPROXY"
docker compose --env-file "$ENV_FILE" build

# --- [6/7] start and wait healthy -------------------------------------------
log "6/7 start stack"
docker compose --env-file "$ENV_FILE" up -d

echo "  waiting for services to become ready (timeout ${TIMEOUT}s)..."
# services with a healthcheck must be "healthy"; services without one (node-web,
# zlmediakit) just need to be running. migrate runs once during up and is not
# waited on here (verified in step 7).
services="$(docker compose --env-file "$ENV_FILE" config --services | grep -v '^migrate$' || true)"
all_ok=1
for svc in $services; do
  ok=0
  for i in $(seq 1 "$TIMEOUT"); do
    line="$(docker compose --env-file "$ENV_FILE" ps -a --format '{{.Service}} {{.State}} {{.Health}}' | awk -v s="$svc" '$1==s {print $2" "$3}' | head -1)"
    state="$(echo "$line" | cut -d' ' -f1)"
    health="$(echo "$line" | cut -d' ' -f2)"
    if [ "$state" = "running" ] && { [ -z "$health" ] || [ "$health" = "healthy" ] || [ "$health" = "<none>" ]; }; then
      echo "  $svc: ok"
      ok=1
      break
    fi
    sleep 1
  done
  if [ "$ok" != "1" ]; then
    echo "  $svc: NOT ready (state=${state:-?} health=${health:-?})" >&2
    docker compose --env-file "$ENV_FILE" logs --tail 50 "$svc" 2>/dev/null || true
    all_ok=0
    break
  fi
done
[ "$all_ok" = "1" ] || die "services not ready within ${TIMEOUT}s"

# --- [7/7] migrate + health check -------------------------------------------
log "7/7 migrate + health check"
docker compose --env-file "$ENV_FILE" run --rm migrate up

HTTP_PORT="$(grep -E '^NV_HTTP_PORT=' "$ENV_PATH" | cut -d= -f2 || echo 8080)"
if curl -fsS -m 5 "http://127.0.0.1:${HTTP_PORT}/api/health" >/dev/null; then
  echo "  health: http://127.0.0.1:${HTTP_PORT}/api/health OK"
else
  die "health endpoint not reachable after deploy"
fi

trap - ERR
ROLLBACK_DONE=1

log "DEPLOY DONE"
echo "  commit: $(git rev-parse --short HEAD) ($GIT_BRANCH)"
docker compose --env-file "$ENV_FILE" ps
echo
echo "remember: open TCP ${HTTP_PORT} (and UDP ${NV_SIP_PORT:-5060} if cameras must register from outside) in the cloud security group."
REMOTE_EOF
then
  echo
  echo "== DEPLOY SUCCESS =="
else
  echo "== DEPLOY FAILED (see remote output above) ==" >&2
  exit 1
fi
