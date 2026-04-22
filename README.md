# Tockr

Tockr is a lightweight Kimai-inspired time tracking app for Raspberry Pi 4B and small teams.

## Run Locally

```sh
go run ./cmd/app
```

Default seed login:

- `admin@example.com`
- `admin12345`

Set `TOCKR_SESSION_SECRET` and `TOCKR_ADMIN_PASSWORD` before production use.

## Docker

Use the published GitHub Container Registry image on a Raspberry Pi or server:

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

Then open `http://localhost:8029`.

For local development from source:

```sh
docker compose up --build
```

To update a published-image install:

```sh
docker pull ghcr.io/<owner>/tockr:latest
docker rm -f tockr
docker run -d --name tockr --restart unless-stopped -p 8029:8080 -v tockr-data:/app/data ghcr.io/<owner>/tockr:latest
```

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
- [CI/CD pipeline](docs/ci-cd.md)
