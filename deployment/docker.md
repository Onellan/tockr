# Docker Deployment

## Pull the published image

Tagged builds from `main` and release tags publish to GitHub Container Registry:

```sh
docker pull ghcr.io/<owner>/tockr:latest
docker run -d --name tockr \
  --restart unless-stopped \
  -p 8029:8080 \
  -v tockr-data:/app/data \
  -e TOCKR_SESSION_SECRET='change-this-32-byte-production-secret' \
  -e TOCKR_ADMIN_PASSWORD='change-this-admin-password' \
  ghcr.io/<owner>/tockr:latest
```

Open `http://localhost:8029`.

For a public GHCR package, anonymous pulls work. If the package is private, run `docker login ghcr.io` first with an account that can read the package.

## Compose example

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

Update with:

```sh
docker compose pull
docker compose up -d
```

## Build locally

Build and run:

```sh
docker compose up --build
```

Open `http://localhost:8029`.

Default seed credentials from `docker-compose.yml`:

- Email: `admin@example.com`
- Password: `admin12345`

Change `TOCKR_SESSION_SECRET` and `TOCKR_ADMIN_PASSWORD` before production use.

Useful environment variables:

- `TOCKR_ADDR`: bind address inside the container, default `:8080`.
- `TOCKR_DB_PATH`: SQLite database path.
- `TOCKR_DATA_DIR`: invoice/static data directory.
- `TOCKR_TOTP_MODE`: `disabled`, `optional`, or `required`.
- `TOCKR_FUTURE_TIME_POLICY`: `allow`, `deny`, `end_of_day`, or `end_of_week`.

## Reverse Proxy

Terminate TLS in the proxy and forward:

- `X-Forwarded-Proto`
- `X-Forwarded-For`
- `Host`

Set `TOCKR_COOKIE_SECURE=true` when served over HTTPS.
