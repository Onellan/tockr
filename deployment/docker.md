# Docker Deployment

Build and run:

```sh
docker compose up --build
```

Open `http://localhost:8080`.

Default seed credentials from `docker-compose.yml`:

- Email: `admin@example.com`
- Password: `admin12345`

Change `TOCKR_SESSION_SECRET` and `TOCKR_ADMIN_PASSWORD` before production use.

## Reverse Proxy

Terminate TLS in the proxy and forward:

- `X-Forwarded-Proto`
- `X-Forwarded-For`
- `Host`

Set `TOCKR_COOKIE_SECURE=true` when served over HTTPS.

