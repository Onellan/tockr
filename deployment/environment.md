# Environment Variables

| Variable | Default | Description |
|---|---|---|
| `TOCKR_ADDR` | `:8080` | HTTP listen address. |
| `TOCKR_DB_PATH` | `data/tockr.db` | SQLite database path. |
| `TOCKR_DATA_DIR` | `data` | Invoice documents and app data. |
| `TOCKR_SESSION_SECRET` | random at startup | HMAC secret for session cookies. Must be fixed in production. |
| `TOCKR_COOKIE_SECURE` | `false` | Set `true` behind HTTPS. |
| `TOCKR_DEFAULT_TIMEZONE` | `UTC` | Default timezone for seeded data. |
| `TOCKR_DEFAULT_CURRENCY` | `USD` | Default currency. |
| `TOCKR_FUTURE_TIME_POLICY` | `end_of_day` | `allow`, `deny`, `end_of_day`, or `end_of_week`. |
| `TOCKR_ADMIN_EMAIL` | `admin@example.com` | First admin email. |
| `TOCKR_ADMIN_PASSWORD` | `admin12345` | First admin password. Change immediately. |
| `TOCKR_WEBHOOK_MAX_RETRIES` | `5` | Maximum webhook delivery attempts. |

