# Docker Deployment

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
