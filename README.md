# Tockr

Tockr is a lightweight Kimai-inspired time tracking app for Raspberry Pi 4B and small teams.

## Run Locally

```sh
go run ./cmd/app
```

Default local seed login:

- `admin@example.com`
- `admin12345`

The local development fallback is intentionally simple. Docker installs
generate and persist the bootstrap password automatically.

## Docker

Use the published GitHub Container Registry image on a Raspberry Pi or any server:

```sh
docker volume create tockr-data
docker run -d --name tockr \
  --restart unless-stopped \
  -p 8029:8080 \
  -v tockr-data:/app/data \
  ghcr.io/onellan/tockr:latest
```

Open `http://localhost:8029` after retrieving the generated bootstrap password.

Retrieve the generated bootstrap password:

```sh
docker exec tockr cat /app/data/.admin_password
```

Log in with:

- Email: `admin@example.com`
- Password: the generated value printed by the command above

The session secret and bootstrap admin password are both generated on first
start and persisted in the data volume. No manual secret creation is required.

For local development from source:

```sh
docker compose up --build
```

To update a published-image install:

```sh
docker pull ghcr.io/onellan/tockr:latest
docker rm -f tockr
docker run -d --name tockr --restart unless-stopped \
  -p 8029:8080 -v tockr-data:/app/data \
  ghcr.io/onellan/tockr:latest
```

For a full step-by-step guide see [docs/docker-setup.md](docs/docker-setup.md).

## Features

- Login/logout with secure signed session cookies.
- Optional TOTP two-factor authentication with recovery codes via `TOCKR_TOTP_MODE`.
- CSRF protection for mutating routes.
- Users, customers, projects, activities, tags, rates.
- Tasks under projects for more precise tracking.
- Favorite/pinned timer starts for repeated work.
- Timesheet start/stop and manual time entries.
- Read-only weekly calendar for reviewing scoped time entries.
- Account self-service for display name, timezone, password, and TOTP.
- Workspace switcher for users with access to multiple workspaces.
- Organization-level workspace administration, workspace creation, and workspace member management.
- Project membership and group assignment editing for project managers/admins.
- Bulk group/project membership editors.
- Workspace-scoped project templates for repeatable project setup.
- Future-time policy: `allow`, `deny`, `end_of_day`, `end_of_week`.
- Dashboard, project insights, entity reports, task reports, and saved report definitions.
- Effective-dated billable rates and user cost rates for future profitability reporting.
- Basic invoice creation, metadata API, and invoice download.
- Compact JSON APIs with pagination.
- Signed webhook delivery with persisted retry queue.
- SQLite WAL storage and migration utility for legacy Kimai SQLite exports.

## Documentation

- [Architecture](architecture.md)
- [Product tracker](TRACKER.md)
- [Schema](schema.md)
- [Docker deployment](deployment/docker.md)
- [systemd deployment](deployment/systemd.md)
- [Raspberry Pi notes](deployment/raspberry-pi.md)
- [Raspberry Pi Docker install](docs/raspberry-pi.md)
- [Docker configuration](docs/configuration.md)
- [Docker updates](docs/updating.md)
- [Troubleshooting](docs/troubleshooting.md)
- [CI/CD pipeline](docs/ci-cd.md)
