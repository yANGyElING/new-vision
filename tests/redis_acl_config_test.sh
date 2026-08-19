#!/bin/sh
set -eu

compose=compose.yaml
projection=internal/nodeapp/projection.go

line=$(grep -F 'exec redis-server' "$compose")
nodeapp_acl=$(printf '%s\n' "$line" | sed -e 's/.*--user nodeapp //' -e 's/ --user access.*//')
access_acl=$(printf '%s\n' "$line" | sed -e 's/.*--user access //' -e "s/']$//")

expected_nodeapp='on ">$$NV_REDIS_NODEAPP_PASSWORD" ~nv:nodeapp:v1:* +ping +hgetall +hset +del +set +multi +exec +sadd +smembers -flushdb -flushall'
expected_access='on ">$$NV_REDIS_ACCESS_PASSWORD" ~nv:access:v1:* +ping +get +set +del +hget +hgetall +hset +hdel +incr +xadd +xrange +xtrim +multi +exec +discard +watch +unwatch +sadd +smembers +srem -flushdb -flushall'

if [ "$nodeapp_acl" != "$expected_nodeapp" ]; then
    echo "unexpected nodeapp Redis ACL: $nodeapp_acl" >&2
    exit 1
fi
if [ "$access_acl" != "$expected_access" ]; then
    echo "unexpected access Redis ACL: $access_acl" >&2
    exit 1
fi
if grep -Fq '.Keys(' "$projection"; then
    echo 'NodeApp projection must not require the Redis KEYS command' >&2
    exit 1
fi
