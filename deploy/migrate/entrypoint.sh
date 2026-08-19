#!/bin/sh
set -eu

exec migrate -path /migrations -database "$DATABASE_URL" "$@"
