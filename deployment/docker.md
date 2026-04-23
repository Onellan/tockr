# Docker Deployment

See [README.md](../README.md#docker-install).


## Pull the published image

Tagged builds from `main` and release tags publish to GitHub Container Registry:

```sh
docker volume create tockr-data
docker run -d --name tockr \
  --restart unless-stopped \
  -p 8029:8080 \
  -v tockr-data:/app/data \
  ghcr.io/onellan/tockr:latest
```

Open `http://localhost:8029`.

Retrieve the generated admin password:

```sh
docker exec tockr cat /app/data/.admin_password
```

The session secret and bootstrap admin password are generated on first start
and stored in the data volume.

For a private GHCR package, run `docker login ghcr.io` first.

## Compose example (production)

Use `docker-compose.prod.yml` from the repo root:

```sh
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
```

Or copy this into your own `compose.yml`:

```yaml
services:
  tockr:
    image: ghcr.io/onellan/tockr:latest
    ports:
      - "8029:8080"
    environment:
      TOCKR_ADMIN_EMAIL: "admin@example.com"
      TOCKR_DEFAULT_TIMEZONE: "Africa/Johannesburg"
      TOCKR_DEFAULT_CURRENCY: "ZAR"
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

```sh
docker compose up --build
```

Open `http://localhost:8029`.

Default local development email from `docker-compose.yml`:

- Email: `admin@example.com`
- Password: generated at `/app/data/.admin_password`

## Useful environment variables

See [docs/configuration.md](../docs/configuration.md) for the full reference.

- `TOCKR_ADDR`: bind address inside the container (default `:8080`).
- `TOCKR_DB_PATH`: SQLite database path.
- `TOCKR_DATA_DIR`: invoice/static data directory.
- `TOCKR_SESSION_SECRET`: HMAC secret. Auto-generated if unset.
- `TOCKR_TOTP_MODE`: `disabled`, `optional`, or `required`.
- `TOCKR_FUTURE_TIME_POLICY`: `allow`, `deny`, `end_of_day`, or `end_of_week`.

## Reverse Proxy

Terminate TLS in the proxy and forward:

- `X-Forwarded-Proto`
- `X-Forwarded-For`
- `Host`

Set `TOCKR_COOKIE_SECURE=true` when served over HTTPS.
