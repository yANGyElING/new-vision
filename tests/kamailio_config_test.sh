#!/bin/sh
set -eu

cfg=deploy/kamailio/kamailio.cfg
dockerfile=deploy/node-access/Dockerfile
module=deploy/kamailio/modules/gb28181/gb28181.c
compose=compose.yaml

grep -Fq 'ARG KAMAILIO_VERSION=6.1.3' "$dockerfile"
grep -Fq 'COPY deploy/kamailio/modules/gb28181' "$dockerfile"
grep -Fq 'listen=udp:0.0.0.0:5060' "$cfg"
grep -Fq 'listen=tcp:0.0.0.0:8090' "$cfg"
grep -Fq 'loadmodule "gb28181.so"' "$cfg"
grep -Fq 'gb28181_rpc_dispatch();' "$cfg"
grep -Fq 'xhttp_load_api(&xhttp_api)' "$module"
grep -Fq 'json_loadb(' "$module"
grep -Fq 'MODULE_VERSION' "$module"
grep -Fq 'xmlReadMemory' "$module"
grep -Fq 'gb28181_record_registration();' "$cfg"
grep -Fq 'pv_auth_check' "$cfg"
grep -Fq 'auth_challenge' "$cfg"
grep -Fq 'reg_send_reply();' "$cfg"
grep -Fq 'register_timer(access_timer' "$module"
grep -Fq 'JSON_REJECT_DUPLICATES' "$module"
grep -Fq '#define RPC_BODY_MAX (16 * 1024 * 1024)' "$module"
for method in applyDeviceProfile removeDeviceProfile replaceDeviceProfiles getRuntimeSnapshot pollEvents ackEvents; do
    grep -Fq "access.v1.$method" "$module"
done
for behavior in PROFILE_VERSION_CONFLICT XRANGE XTRIM WATCH MULTI EXEC DISCARD; do
    grep -Fq "$behavior" "$module"
done
if grep -Eq 'jsonrpc_dispatch|rpc_unavailable|rpc_register_array' "$cfg" "$module"; then
    echo 'nested Access JSON-RPC must use the module-owned xhttp dispatcher' >&2
    exit 1
fi
grep -Fq 'LIBS=-lhiredis -ljansson' deploy/kamailio/modules/gb28181/Makefile
grep -Fq 'pkg-config --cflags libxml-2.0' deploy/kamailio/modules/gb28181/Makefile
grep -Fq '"${NV_SIP_PORT}:5060/udp"' "$compose"

if grep -A30 '^  node-access:' "$compose" | grep -Eq '8090:|8080:'; then
    echo 'node-access HTTP must not be published to the host' >&2
    exit 1
fi
