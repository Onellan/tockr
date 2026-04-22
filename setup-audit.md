# Docker Setup Audit

## Current State

- Application stack: Go web server with server-rendered templates and SQLite.
- Runtime image: multi-stage Docker build, Alpine runtime, non-root UID/GID
  `65532`, `/healthz` healthcheck, static assets copied into `/app/web/static`.
- Persistent data: `/app/data`, normally mounted as Docker volume
  `tockr-data`.
- Database initialization: SQLite parent directory is created automatically;
  schema migrations run on app startup.
- First admin bootstrap: `SeedAdmin` creates the first admin user only when
  the users table is empty.
- Existing automation: Docker entrypoint already generated and persisted
  `TOCKR_SESSION_SECRET`.

## Gaps Found

- Docker docs still used a known default admin password.
- Production Compose set `TOCKR_ADMIN_PASSWORD` explicitly instead of letting
  the install generate it.
- CI smoke test supplied secrets instead of validating the real automated
  install path.
- `docs/troubleshooting.md` referenced `docs/updating.md`, but that file did
  not exist.
- Docker setup documentation was spread across README, `deployment/`, and
  `docs/`, with repeated credential instructions.

## Blockers To One-Pass Install

- A first-time Docker user needed either to accept a known password or invent a
  replacement value. This has been replaced with generated password bootstrap.
- Published image validation needed to prove generated credentials work, not
  only that `/healthz` responds.
