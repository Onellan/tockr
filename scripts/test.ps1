$ErrorActionPreference = "Stop"

docker compose version | Out-Null
docker compose run --build --rm test
