#!/usr/bin/env bash
# temp helper: SQL-clean leftover e2e data + verify audit logs
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
cd /opt/new-vision

echo "=== cleanup e2e rows ==="
docker compose --env-file nvenv exec -T postgres psql -U new_vision -d new_vision <<'SQL'
DELETE FROM user_region_scopes WHERE user_id IN (SELECT id FROM users WHERE username LIKE 'e2e-%');
DELETE FROM user_roles WHERE user_id IN (SELECT id FROM users WHERE username LIKE 'e2e-%');
DELETE FROM users WHERE username LIKE 'e2e-%';
DELETE FROM tenants WHERE name LIKE 'e2e-%';
SELECT name FROM tenants ORDER BY created_at;
SQL

echo "=== audit summary ==="
docker compose --env-file nvenv exec -T postgres psql -U new_vision -d new_vision <<'SQL'
SELECT action, result, count(*) AS n FROM audit_logs GROUP BY action, result ORDER BY action, result;
SQL

echo "=== recent denied logins ==="
docker compose --env-file nvenv exec -T postgres psql -U new_vision -d new_vision <<'SQL'
SELECT action, result, ip_addr, detail FROM audit_logs WHERE result = 'denied' ORDER BY created_at DESC LIMIT 3;
SQL
EOF
