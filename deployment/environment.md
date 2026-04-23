# Environment Variables

See [README.md](../README.md#configuration).

|---|---|---|
| `TOCKR_ADDR` | `:8029` | HTTP listen address for local app runs. Docker overrides this to `:8080` inside the container and publishes host port `8029`. |
| `TOCKR_DB_PATH` | `data/tockr.db` | SQLite database path. |
| `TOCKR_DATA_DIR` | `data` | Invoice documents and app data. |
| `TOCKR_SESSION_SECRET` | auto-generated | HMAC secret for session cookies. Auto-generated on first start and persisted in the data volume. Set explicitly only if you need a fixed value (e.g., multiple replicas or key rotation). |
| `TOCKR_COOKIE_SECURE` | `false` | Set `true` behind HTTPS. |
| `TOCKR_DEFAULT_TIMEZONE` | `UTC` | Default timezone for seeded data. |
| `TOCKR_DEFAULT_CURRENCY` | `ZAR` | Default billing unit for new workspaces. |
| `TOCKR_FUTURE_TIME_POLICY` | `end_of_day` | `allow`, `deny`, `end_of_day`, or `end_of_week`. |
| `TOCKR_ADMIN_EMAIL` | `admin@example.com` | First admin email. |
| `TOCKR_ADMIN_PASSWORD` | auto-generated in Docker, `admin12345` for local app runs | First admin password. Docker stores the generated value at `/app/data/.admin_password`. |
| `TOCKR_WEBHOOK_MAX_RETRIES` | `5` | Maximum webhook delivery attempts. |
