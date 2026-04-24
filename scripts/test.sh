#!/usr/bin/env sh
set -eu

docker compose version >/dev/null
docker compose run --build --rm test
