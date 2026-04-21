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
- CSRF protection for mutating routes.
- Users, customers, projects, activities, tags, rates.
- Timesheet start/stop and manual time entries.
- Future-time policy: `allow`, `deny`, `end_of_day`, `end_of_week`.
- Dashboard and entity reports.
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
