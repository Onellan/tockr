# Tockr Architecture

## 1. Overview
Tockr is a multi-workspace, server-rendered time tracking and billing system focused on engineering delivery teams.

Primary use case:
- Capture time against Client -> Project -> Workstream -> Deliverable -> Task.
- Apply workspace-scoped billing rules (rates, exchange rates, recalculation).
- Convert unexported billable time into invoices and operational reporting.

## 2. High-Level Architecture
Tockr is a Go modular monolith with an in-process webhook worker and SQLite persistence.

Text diagram:

```
Browser (HTML forms + minimal JS)
				|
				v
Go HTTP Server (chi + net/http)
	- middleware: session, CSRF, security headers, rate limiting
	- handlers: UI + API routes
	- templ server-side rendering
				|
				v
SQLite Store (modernc.org/sqlite)
	- schema + migrations + repositories
	- workspace scoping in queries
				|
				+--> In-process webhook queue/worker -> external endpoints
				+--> SMTP sender (per-workspace config, optional global fallback)
```

## 3. Technology Stack
Backend:
- Go 1.26
- net/http + github.com/go-chi/chi/v5
- templ for server-side component rendering
- modernc.org/sqlite driver

Frontend:
- Server-rendered HTML
- Plain JavaScript (`web/static/menu.js`) for nav collapse, dropdowns, dependent selectors, mobile nav, and UI helpers
- CSS from `web/static/style.css`

Database:
- SQLite (WAL mode, foreign keys ON, busy timeout)
- Single DB connection (`SetMaxOpenConns(1)`) for predictable behavior on low-resource hosts

Runtime / Raspberry Pi considerations:
- CGO disabled binaries, small Alpine runtime image
- Multi-arch Docker image (`amd64`, `arm64`, `arm/v7`, `arm/v6`) in CI
- Persistent volume/bind-mount for DB, invoice files, and generated secrets

## 4. Application Structure
Top-level implementation layout:

- `cmd/app`: production entrypoint, DB open/migrate/seed, HTTP server startup, graceful shutdown, webhook worker
- `cmd/demo-seed`: demo data seeding utility
- `cmd/migrate`: legacy migration utility
- `internal/platform/http`: routing, middleware, handlers, UI/API orchestration
- `internal/db/sqlite`: schema, migrations, repository/query logic
- `internal/auth`: password hashing, TOTP, permissions
- `internal/domain`: shared data contracts/entities
- `internal/platform/config`: env config loading
- `internal/platform/email`: SMTP sender abstraction
- `internal/webhooks`: queue drain and signed delivery retries
- `web/templates`: templ-based page components
- `web/static`: assets and JS behavior

## 5. Data Model
Core chain used by timesheets:

```
Client (customers)
	-> Project (projects)
			-> Workstream (workstreams via project_workstreams)
			-> Deliverable (activities)
					-> Task (tasks)
							-> Timesheet (timesheets)
```

Important modeling details from schema/handlers:
- `projects.customer_id` required.
- `tasks.project_id` required.
- `activities.project_id` is nullable: supports global deliverables and project-bound deliverables.
- `timesheets` includes `workspace_id`, `customer_id`, `project_id`, optional `workstream_id`, `activity_id`, optional `task_id`.
- Rates are workspace-scoped and can be broad or specific using nullable scope columns (`customer_id`, `project_id`, `activity_id`, `task_id`, `user_id`).
- Exchange rates are workspace-scoped (`exchange_rates.workspace_id`).
- Work schedule is now workspace-scoped (`workspace_work_schedules`).
- Multi-tenant boundaries are enforced by `workspace_id` on operational tables and workspace-aware access context.

## 6. Key Workflows
### Time capture
- `/timesheets` supports manual duration and start/end modes.
- Optional live timer via `/timesheets/start` and `/timesheets/stop`.
- Dependent selector behavior in JS prevents stale hidden child select values.
- Edit is dedicated page `/timesheets/{id}/edit`.

### Project creation (multi-step)
- Route: `/projects/create`.
- Steps: `details -> workstreams -> activities (deliverables) -> users`.
- Draft stored in signed cookie (`tockr_project_create_draft`) until submit.
- Final submit creates project and related records from draft atomically through store workflow.

### Reporting / dashboard aggregation
- Dashboard summary from `store.Dashboard(access)` (workspace-scoped, role-aware).
- Reports (`/reports`) support grouping and filters with saved report presets and share tokens.
- Utilization (`/reports/utilization`) computes expected time using workspace work schedule.
- CSV exports for reports and timesheets.

### User / workspace management
- Organization/workspace admin screens under `/admin/workspaces`, `/admin/users`, `/admin/...`.
- Session includes active `workspace_id`; switching via `POST /workspace` updates session scope.
- Workspace membership and role assignments control access boundaries.

## 7. API Design
API namespace: `/api/*`.

Main endpoints:
- `GET /api/status`
- `GET /api/customers`
- `GET /api/projects`
- `GET /api/activities`
- `GET /api/tasks`
- `GET /api/timesheets`
- `POST /api/timer/start`
- `POST /api/timer/stop`
- `GET /api/invoices/{id}/download`
- `PATCH /api/invoices/{id}/meta`
- `GET /api/webhooks`

Request/response patterns:
- List endpoints return `{ "data": [...], "page": { ... } }` using `page`/`size` query params (defaults: page=1, size=25).
- JSON responses use `writeJSON` helper.
- API routes reuse same business handlers as UI in several cases (e.g., timer start/stop).

## 8. Authentication & Authorization
Authentication model:
- Cookie-backed signed session (`tockr_session`), persisted in DB `sessions` table.
- Login with password; optional TOTP and optional email OTP challenge.
- Password reset via one-time token flow.

Authorization model:
- Access context includes `organization_id`, `workspace_id`, `workspace_role`, managed/member project sets.
- Permission checks centralized in `internal/auth/permissions.go` and enforced by handler wrappers.
- Workspace switch updates session workspace and all scoped queries follow the active workspace.

## 9. Email System
Design:
- Per-workspace SMTP config stored on workspace record.
- `senderForWorkspace` resolves workspace SMTP; optional global fallback controlled by `TOCKR_SMTP_GLOBAL_FALLBACK`.

Primary email triggers:
- Login email OTP challenge.
- Account email-change OTP verification.
- Password reset link.
- Admin SMTP test sends from admin screens.

## 10. Security
Implemented controls:
- Security headers middleware:
	- `X-Frame-Options: DENY`
	- `X-Content-Type-Options: nosniff`
	- CSP with restrictive defaults
	- HSTS when HTTPS/cookie secure context is active
- CSRF validation for state-changing routes (header token or form token), tied to session CSRF value.
- Access control enforced by workspace/organization checks and permission middleware.
- Rate limiting for sensitive POST routes (login, reset, timer endpoints, account security actions).
- Secure cookie attributes (`HttpOnly`, `SameSite=Lax`, optional `Secure`).
- Audit logging for security- and admin-sensitive actions.

## 11. Deployment Architecture
Container architecture:
- Multi-stage Dockerfile:
	- `deps`
	- `test-runner`
	- `build`
	- runtime Alpine image with non-root UID/GID `65532`
- Entrypoint generates and persists session secret + bootstrap admin password if unset.

Runtime deployment options:
- Local/dev: `docker-compose.yml` with optional Mailpit.
- Production: `docker-compose.prod.yml` using GHCR image.
- Raspberry Pi production pattern: `deployment/docker-compose.yml` bound to `/srv/tockr/data`, often paired with host `cloudflared` tunnel.

## 12. CI/CD
GitHub Actions workflow: `.github/workflows/ci.yml`

Pipeline stages:
- Validate: Docker-based tests (`docker compose run --build --rm test`)
- Security: `gosec`, `go vet`, `go mod tidy` check, `govulncheck`, `gitleaks`
- Container smoke: build image and health/login smoke checks
- Docker publish: buildx multi-platform image and push to GHCR on `main` and version tags

## 13. Performance Considerations
Current implementation posture:
- SQLite + WAL, single open connection; suitable for small teams and simple ops.
- Server-rendered pages reduce frontend runtime overhead.
- Pagination defaults prevent unbounded list rendering.
- Aggregation-heavy views (dashboard/reports/utilization) execute SQL at request time.

Practical concurrency assumptions:
- System is tuned for low-to-moderate concurrent usage (for example, tens of active users such as ~30), not high-write parallel workloads.
- Single DB connection is intentional for stability on constrained hosts (including Raspberry Pi), but limits write concurrency.

## 14. Future Scalability
If scaling beyond current target profile, likely changes:
- Move from SQLite to networked RDBMS (PostgreSQL/MySQL) for concurrent write throughput.
- Introduce background job processing for heavy recalculations/report pre-aggregation and webhook dispatch.
- Add cache layer for repeated dashboard/report queries.
- Introduce API auth tokens/service accounts for external integrations (current API uses session/cookie auth).
- Consider read/write separation and horizontal app replicas with centralized session strategy.

Known limitations in current design:
- Session-based API auth is browser-centric.
- Some operational settings remain global (`settings` table items like email policy/workspace secret key), while core billing/schedule controls are workspace-scoped.
- Synchronous request-time aggregation can become expensive on very large timesheet volumes.
