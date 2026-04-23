# Raspberry Pi Docker Install

See [README.md](../README.md#raspberry-pi).


## Supported Platform

- Recommended: Raspberry Pi 4B or newer.
- Recommended OS: Raspberry Pi OS 64-bit.
- Published image platforms: `linux/arm64` and `linux/amd64`.
- Default host port: `8029`.
- Persistent data: Docker volume `tockr-data`.

## Quick Install

```sh
docker volume create tockr-data
docker run -d --name tockr \
  --restart unless-stopped \
  -p 8029:8080 \
  -v tockr-data:/app/data \
  ghcr.io/onellan/tockr:latest
```

Retrieve the generated admin password:

```sh
docker exec tockr cat /app/data/.admin_password
```

Open:

```text
http://<pi-hostname-or-ip>:8029
```

Log in with:

- Email: `admin@example.com`
- Password: the generated value from `/app/data/.admin_password`

The session secret and bootstrap admin password are generated on first start
and persisted in the Docker volume. You do not need to create configuration
values manually.

## Compose Install

```sh
curl -fsSL https://raw.githubusercontent.com/Onellan/tockr/main/docker-compose.prod.yml \
  -o docker-compose.prod.yml
docker compose -f docker-compose.prod.yml up -d
docker compose -f docker-compose.prod.yml exec tockr cat /app/data/.admin_password
```

## Validate

```sh
docker ps --filter name=tockr
curl -fsS http://<pi-hostname-or-ip>:8029/healthz
docker logs tockr --tail 50
```

Expected health response:

```json
{"status":"ok"}
```

## Update

Docker run:

```sh
docker pull ghcr.io/onellan/tockr:latest
docker rm -f tockr
docker run -d --name tockr \
  --restart unless-stopped \
  -p 8029:8080 \
  -v tockr-data:/app/data \
  ghcr.io/onellan/tockr:latest
```

Compose:

```sh
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
```

## Backup

```sh
docker stop tockr
docker run --rm -v tockr-data:/data -v "$PWD":/backup alpine \
  tar -czf "/backup/tockr-backup-$(date +%F).tgz" -C /data .
docker start tockr
```

For the full setup guide, see [docker-setup.md](docker-setup.md). For updates
and restore steps, see [updating.md](updating.md).
