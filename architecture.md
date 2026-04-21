# Architecture

Tockr is a lightweight Kimai-inspired time tracking application designed for Raspberry Pi 4B deployments. It is a Go modular monolith with server-rendered HTML, SQLite WAL storage, secure cookie-backed sessions, and a small integration API.

## Runtime Shape

- Single Go binary.
- `chi` router and standard `net/http` server.
- `templ` runtime components for HTML rendering.
- SQLite with WAL, foreign keys, busy timeout, and short transactions.
- No SPA, no Node.js runtime, no microservices.
- htmx-compatible markup is used for progressive enhancement only; all primary workflows work as normal HTML forms.

## Modules

- `internal/auth`: login, logout, password hashing, role/permission checks, sessions.
- `internal/users`: user administration.
- `internal/customers`, `internal/projects`, `internal/activities`: master data.
- `internal/timesheets`: timer start/stop, manual time entry, filtering, tagging, future-time policy.
- `internal/rates`: simple scoped rate resolution.
- `internal/reports`: dashboard, customer, activity, project, and user rollups.
- `internal/invoices`: invoice records, invoice metadata, CSV export, invoice document download.
- `internal/webhooks`: signed JSON webhooks with in-process retry.
- `internal/api`: compact JSON integration API.
- `internal/db/sqlite`: migrations, repositories, and SQLite-specific pragmas.

## Pi-Friendly Decisions

- Money is stored as integer cents.
- Timestamps are stored as UTC RFC3339 text.
- Lists are paginated by default.
- Reports are aggregation queries, not background materialization.
- Webhooks run in-process and persist delivery attempts for retry.
- The app avoids ORM reflection and heavyweight template engines.

