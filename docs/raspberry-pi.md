# Raspberry Pi Docker Install

Tockr publishes a Raspberry Pi friendly image to GitHub Container Registry.

## Supported Pi platform

- Recommended OS: Raspberry Pi OS 64-bit.
- Required image platform: `linux/arm64`.
- Also published: `linux/amd64` for development and server installs.
- Not published by default: `linux/arm/v7`. The project targets Raspberry Pi 4B class 64-bit installs to keep CI fast and the release matrix small.

## Install

Replace `<owner>` with the GitHub owner that publishes this repository.

```sh
docker pull ghcr.io/<owner>/tockr:latest
docker volume create tockr-data
docker run -d --name tockr \
  --restart unless-stopped \
  -p 8029:8080 \
  -v tockr-data:/app/data \
  -e TOCKR_SESSION_SECRET='change-this-32-byte-production-secret' \
  -e TOCKR_ADMIN_EMAIL='admin@example.com' \
  -e TOCKR_ADMIN_PASSWORD='change-this-admin-password' \
  -e TOCKR_DEFAULT_TIMEZONE='UTC' \
  -e TOCKR_DEFAULT_CURRENCY='USD' \
  -e TOCKR_FUTURE_TIME_POLICY='end_of_day' \
  -e TOCKR_TOTP_MODE='disabled' \
  ghcr.io/<owner>/tockr:latest
```

Open:

```text
http://<pi-hostname-or-ip>:8029
```

## Compose

```yaml
services:
  tockr:
    image: ghcr.io/<owner>/tockr:latest
    ports:
      - "8029:8080"
    environment:
      TOCKR_ADDR: ":8080"
      TOCKR_DB_PATH: "/app/data/tockr.db"
      TOCKR_DATA_DIR: "/app/data"
      TOCKR_SESSION_SECRET: "change-this-32-byte-production-secret"
      TOCKR_ADMIN_EMAIL: "admin@example.com"
      TOCKR_ADMIN_PASSWORD: "change-this-admin-password"
      TOCKR_DEFAULT_TIMEZONE: "UTC"
      TOCKR_DEFAULT_CURRENCY: "USD"
      TOCKR_FUTURE_TIME_POLICY: "end_of_day"
      TOCKR_TOTP_MODE: "disabled"
    volumes:
      - tockr-data:/app/data
    restart: unless-stopped

volumes:
  tockr-data:
```

## Update

```sh
docker pull ghcr.io/<owner>/tockr:latest
docker stop tockr
docker rm tockr
# Re-run the install command with the same volume and environment.
```

With Compose:

```sh
docker compose pull
docker compose up -d
```

## Backup

For a simple volume backup, stop the container briefly and archive the data volume:

```sh
docker stop tockr
docker run --rm -v tockr-data:/data -v "$PWD":/backup alpine \
  tar -czf "/backup/tockr-data-$(date +%F).tgz" -C /data .
docker start tockr
```
