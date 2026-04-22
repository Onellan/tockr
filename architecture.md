# Architecture

Tockr is a lightweight Kimai-inspired time tracking application designed for Raspberry Pi 4B deployments. It is a Go modular monolith with server-rendered HTML, SQLite WAL storage, secure cookie-backed sessions, and a small integration API.

## Runtime Shape

- Single Go binary.
- `chi` router and standard `net/http` server.
- `templ` runtime components for HTML rendering.
- SQLite with WAL, foreign keys, busy timeout, and short transactions.
- No SPA, no Node.js runtime, no microservices.
- A small plain JavaScript file handles navigation, dropdown menus, and dependent selectors; all primary workflows work as normal HTML forms.

## Modules

- `cmd/app`: application entrypoint, graceful shutdown, database setup, and webhook worker startup.
- `cmd/migrate`: one-way legacy Kimai SQLite import utility.
- `internal/auth`: password hashing, optional TOTP, role constants, and permission checks.
- `internal/domain`: core data types shared across HTTP, persistence, and integrations.
- `internal/platform/config`: environment-backed runtime configuration.
- `internal/platform/http`: routes, handlers, middleware, request validation, and API endpoints.
- `internal/db/sqlite`: migrations, repositories, hierarchy backfill, and SQLite-specific pragmas.
- `internal/db/legacy`: legacy role and value conversion helpers for imports.
- `internal/webhooks`: signed JSON webhook delivery with persisted retry state.
- `web/templates`: server-rendered HTML components.
- `web/static`: CSS, favicon assets, and the small JavaScript enhancement layer.

## Pi-Friendly Decisions

- Money is stored as integer cents.
- Timestamps are stored as UTC RFC3339 text.
- Lists are paginated by default.
- Reports are aggregation queries, not background materialization.
- Calendar is a read-only weekly projection over existing timesheet queries.
- Webhooks run in-process and persist delivery attempts for retry.
- The app avoids ORM reflection and heavyweight template engines.
