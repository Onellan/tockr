# Configuration Reference

Tockr is configured with environment variables. Docker installs work without
setting any variables because secrets and the bootstrap admin password are
generated automatically when missing.

## Docker Defaults

| Variable | Default | Automated behavior | When to change |
|---|---|---|---|
| `TOCKR_ADDR` | `:8080` in Docker | Set by the image. | Usually never; change the host port mapping instead. |
| `TOCKR_DB_PATH` | `/app/data/tockr.db` | Parent directory is created on startup. | Only for custom storage layouts. |
| `TOCKR_DATA_DIR` | `/app/data` | Used for generated secrets, database, and files. | Only for custom storage layouts. |
| `TOCKR_SESSION_SECRET` | Generated | Stored at `/app/data/.session_secret`. | Only for key rotation or multiple replicas. |
| `TOCKR_ADMIN_EMAIL` | `admin@example.com` | Used when seeding the first admin user. | Optional before first start. |
| `TOCKR_ADMIN_PASSWORD` | Generated | Stored at `/app/data/.admin_password`. | Optional before first start if you require a chosen password. |
| `TOCKR_DEFAULT_TIMEZONE` | `UTC` | Used for first-run seeded data. | Optional before first start. |
| `TOCKR_DEFAULT_CURRENCY` | `USD` | Used for first-run seeded data. | Optional before first start. |
| `TOCKR_FUTURE_TIME_POLICY` | `end_of_day` | Applied by the app. | Optional. |
| `TOCKR_TOTP_MODE` | `disabled` | Applied by the app. | Set to `optional` or `required` for 2FA. |
| `TOCKR_COOKIE_SECURE` | `false` | Applied to session cookies. | Set `true` behind HTTPS. |
| `TOCKR_WEBHOOK_MAX_RETRIES` | `5` | Applied by the webhook worker. | Rarely. |

## Retrieve Generated Values

Docker run:

```sh
docker exec tockr cat /app/data/.admin_password
docker exec tockr test -s /app/data/.session_secret
```

Docker Compose:

```sh
docker compose -f docker-compose.prod.yml exec tockr cat /app/data/.admin_password
docker compose -f docker-compose.prod.yml exec tockr test -s /app/data/.session_secret
```

The session secret is intentionally not printed. It only needs to exist and
stay stable in the volume.

## Local Development Defaults

When running without Docker:

```sh
go run ./cmd/app
```

Defaults are:

| Variable | Local default |
|---|---|
| `TOCKR_ADDR` | `:8029` |
| `TOCKR_DB_PATH` | `data/tockr.db` |
| `TOCKR_DATA_DIR` | `data` |
| `TOCKR_ADMIN_EMAIL` | `admin@example.com` |
| `TOCKR_ADMIN_PASSWORD` | `admin12345` |

Local defaults are for development only. Docker is the recommended self-hosted
install path because it persists generated secrets automatically.

## Variable Details

### `TOCKR_ADDR`

The HTTP listen address. In Docker, leave this as `:8080` and publish a host
port with Docker, for example `-p 8029:8080`.

### `TOCKR_DB_PATH`

Path to the SQLite database. In Docker this should remain under `/app/data` so
it is stored in the persistent volume.

### `TOCKR_DATA_DIR`

Directory for generated secrets, SQLite data, and app-generated files. In
Docker this should be mounted as a named volume.

### `TOCKR_SESSION_SECRET`

HMAC key for signed session cookies. The Docker entrypoint generates a 64
character hex value if this variable is empty and stores it in the data volume.
Changing it invalidates existing sessions.

### `TOCKR_ADMIN_EMAIL`

Email for the first admin user. It is only used when the users table is empty.
Changing it after first boot does not rename an existing user.

### `TOCKR_ADMIN_PASSWORD`

Password for the first admin user. In Docker it is generated if omitted and
stored in `/app/data/.admin_password`. It is only used when the users table is
empty. If the admin changes their password in the UI later, the file is no
longer the login password.

### `TOCKR_DEFAULT_TIMEZONE`

IANA timezone used for first-run seeded data, for example `Africa/Johannesburg`
or `Europe/London`.

### `TOCKR_DEFAULT_CURRENCY`

ISO 4217 currency code used for first-run seeded data, for example `ZAR`,
`USD`, `EUR`, or `GBP`.

### `TOCKR_FUTURE_TIME_POLICY`

Controls future time entry:

| Value | Behavior |
|---|---|
| `allow` | Future entries are allowed. |
| `deny` | Future entries are blocked. |
| `end_of_day` | Entries are allowed through the end of the current day. |
| `end_of_week` | Entries are allowed through the end of the current week. |

### `TOCKR_TOTP_MODE`

Controls two-factor authentication:

| Value | Behavior |
|---|---|
| `disabled` | TOTP is unavailable. |
| `optional` | Users may enroll TOTP. |
| `required` | Users must enroll TOTP before using the app. |

### `TOCKR_COOKIE_SECURE`

Set to `true` when the app is served through HTTPS. Leave `false` for direct
HTTP access on a private LAN during initial setup.

### `TOCKR_WEBHOOK_MAX_RETRIES`

Maximum retry attempts for outgoing webhook deliveries.
