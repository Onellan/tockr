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

```sh
docker compose up --build
```

Open `http://localhost:8029`.

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
- Project membership and group assignment editing for project managers/admins.
- Future-time policy: `allow`, `deny`, `end_of_day`, `end_of_week`.
- Dashboard, project insights, entity reports, task reports, and saved report definitions.
- Effective-dated billable rates and user cost rates for future profitability reporting.
- Basic invoice creation, metadata API, and invoice download.
- Compact JSON APIs with pagination.
- Signed webhook delivery with persisted retry queue.
- SQLite WAL storage and migration utility for legacy Kimai SQLite exports.

## Documentation

- [Architecture](architecture.md)
- [Rewrite plan](rewrite-plan.md)
- [Schema](schema.md)
- [Migration plan](migration-plan.md)
- [Docker deployment](deployment/docker.md)
- [systemd deployment](deployment/systemd.md)
- [Raspberry Pi notes](deployment/raspberry-pi.md)
