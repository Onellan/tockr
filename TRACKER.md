# Tockr Product Tracker

This file replaces the scattered planning, audit, impact, and follow-up markdown files that used to live at the repository root and under `docs/`. Keep this file current when features are added, deferred, or intentionally dropped.

## Product Principles

- Keep Tockr lightweight for Raspberry Pi 4B and small-team self-hosting.
- Prefer server-rendered HTML, SQLite, one Go binary, and minimal JavaScript.
- Keep workflows practical and dense rather than marketing-style.
- Preserve existing data, routes, configuration, and deployment flows.
- Add features only when they fit the current architecture cleanly.
- Keep backend authorization authoritative; UI hiding is convenience only.
- Use additive schema changes and safe defaults for upgrades.

## Current Architecture

- Runtime: Go 1.26, `chi`, standard `net/http`, `templ` runtime components, SQLite WAL, and a small static JavaScript layer.
- Storage: SQLite first, integer cents for money, UTC RFC3339 timestamps, booleans as `0`/`1`.
- Deployment: local binary, Docker Compose, systemd, Raspberry Pi 64-bit, and GHCR-published Docker images.
- UI: server-rendered forms and tables with grouped sidebar navigation, account dropdown, saved report dropdown, dependent selectors, and accessible native controls.
- Integration surface: compact JSON API, invoice download/meta endpoints, and signed webhooks with persisted retries.

## Implemented

### Core App

- Login/logout with secure signed session cookies.
- CSRF protection for mutating routes.
- Seed admin account with environment-configurable email/password.
- Account page for display name, timezone, password change, optional TOTP setup, and recovery codes.
- TOTP modes: `disabled`, `optional`, `required`.
- Graceful shutdown and JSON logs.

### Workspace, Roles, And Permissions

- Organization, workspace, group, project member, and project group schema.
- Default organization/workspace backfill for existing data.
- Session-scoped workspace switching.
- Organization-level workspace list/detail administration.
- Workspace creation, editing, archiving, and member role management.
- Workspace/project scoped authorization for UI, handlers, and repository queries.
- Project membership editor for users and groups.
- Bulk workspace group membership editing.
- Bulk project user/group membership editing.
- Private project visibility through direct or group membership.

### Work Tracking

- Customers, projects, activities, tags, tasks, groups, users, rates, invoices, reports, and webhooks.
- Timer start/stop and manual time entries.
- Favorites/pinned entries for repeated timer starts.
- Tasks under projects with task-aware timesheets and reporting.
- Read-only weekly calendar.
- Future-time policies: `allow`, `deny`, `end_of_day`, `end_of_week`.
- Workspace-scoped project templates for repeatable project setup.
- Template-based project creation that copies project defaults, tasks, and activities.

### Reporting And Billing

- Dashboard summary cards.
- Project dashboard with tracked time, billable value, estimate progress, budget progress, and alert threshold state.
- Reports grouped by user, customer, project, activity, task, and group.
- Report filters for date, customer, project, activity, task, user, and group.
- Saved report creation/list/open through the reports page.
- Effective-dated billable rates with customer/project/activity/task/user scopes.
- Effective-dated user cost rates.
- Timesheet creation stores resolved billing and internal cost rates for audit stability.
- Basic invoice creation, metadata API, and invoice download.

### UI And UX

- Grouped sidebar navigation: Work, Manage, Analyze, Admin.
- Collapsible sidebar groups, with Manage, Analyze, and Admin collapsed by default.
- Account dropdown in the topbar.
- Saved report dropdown.
- Project row action dropdown for dashboard/member actions.
- Admin workspace screens for organization owners/admins.
- Group member bulk editor.
- Project template list/create/edit/use screens.
- Favicon, PNG icons, Apple touch icon, manifest, and cache-busted head links.
- Native dropdown selectors instead of raw user-facing ID inputs.
- Human-readable related-record labels in forms and tables.
- Dependent selectors for customer -> project and project -> activity/task.
- Server-side relationship validation for manually posted selector IDs.
- Responsive admin-style layout with panels, tables, badges, and clear focus states.

### CI And Deployment

- GitHub Actions validation, container smoke test, and GHCR publish workflow.
- Docker Buildx multi-platform image for `linux/amd64` and `linux/arm64`.
- Non-root container runtime user.
- Docker healthcheck on `/healthz`.
- Docker build context trimmed for faster Pi-friendly builds.
- Raspberry Pi Docker install documentation.

## Backlog

### High-Value Next Work

- Favorite edit/delete UI.
- Task edit/archive UI.
- Saved report edit/delete UI.
- Saved report sharing UI with signed expiring links.
- Workspace invitations or pending-member states if email onboarding is added.
- More webhook events for newer domain actions such as task, favorite, saved report, membership, and budget/rate changes.
- API fields/endpoints for favorites and saved reports where useful.
- Async/searchable selectors if any user-facing selector regularly exceeds 100 active records.

### Reporting And Finance

- Scheduled report delivery.
- Profitability dashboards using resolved billable and internal cost data.

### Governance

- Timesheet approvals.
- Locked time periods.
- Submit/review workflows for larger teams.
- Audit views for sensitive role, membership, rate, invoice, webhook, and security changes.

### Workspace And Organization Admin

- Organization-level workspace administration can be extended with workspace-level audit views.
- Workspace member management can be extended with invite/pending states if email onboarding is added.
- Future migration inference for restricted/private project visibility from legacy team data.

### Calendar And Time Capture

- Editable calendar drag/drop, only if accidental-edit risk can be handled well.
- Calendar-assisted time entry.
- Browser/desktop auto-tracking remains deferred because it is heavy and privacy-sensitive.

### Infrastructure

- PostgreSQL repository implementation when self-hosted users need it.
- Optional `linux/arm/v7` Docker image only if 32-bit Raspberry Pi installs become a real requirement.
- Path-filter tuning for CI if build frequency becomes noisy.

## Deferred Or Dropped

- Plugin loading and Symfony/Kimai extension parity.
- SAML/LDAP for the Pi-focused MVP.
- OAuth app marketplace.
- Native Jira/QuickBooks/calendar integrations until API/webhooks are not enough.
- Full Office/PDF invoice renderer parity.
- Highly customizable dashboard widgets.
- Heavy frontend build tooling, SPA conversion, or Node.js runtime.
- Materialized report aggregates until SQLite query performance proves insufficient.

## Compatibility Notes

- Schema changes should remain additive where possible.
- Existing records are assigned to the default organization/workspace during migration.
- Existing timesheets keep resolved billable/internal rates.
- Existing rate rows are backfilled to `1970-01-01T00:00:00Z` effective start.
- Existing sessions default to the first accessible workspace when needed.
- Hidden IDs remain acceptable for internal form submissions, row actions, route path IDs, API identifiers, audit logging, and persistence.
- Project templates intentionally do not copy sensitive live data such as timesheets, invoices, rates, memberships, favorites, or audit entries.

## Role Model

| Scope | Role | Purpose |
| --- | --- | --- |
| Organization | owner | Full organization control and security/audit oversight. |
| Organization | admin | Organization-wide administration and reporting. |
| Workspace | admin | Workspace operations, members, groups, project/customer/activity/tag/rate/invoice/webhook management. |
| Workspace | analyst | Workspace-wide reporting and read-only operational data. |
| Workspace | member | Regular time tracking and own timesheet access. |
| Project | manager | Project membership management and project-scoped reporting/timesheets. |
| Project | member | Track time and view assigned project data. |

Legacy role mapping:

- `superadmin`: organization owner plus default workspace admin.
- `admin`: organization admin plus default workspace admin.
- `teamlead`: workspace analyst/member, promoted to project manager when imported team/project evidence proves scope.
- `user`: workspace member.

## Old Planning File Map

These former files were consolidated here:

- `authorization-audit.md`
- `branch-feature-matrix.md`
- `data-model-changes.md`
- `dropdown-menu-plan.md`
- `dropdown-selector-plan.md`
- `exceptions-list.md`
- `favicon-audit.md`
- `favicon-fix-plan.md`
- `feature-adoption-plan.md`
- `feature-gap-analysis.md`
- `feature-parity-matrix.md`
- `financial-model-plan.md`
- `gap-analysis.md`
- `hierarchy-design.md`
- `hierarchy-ui-plan.md`
- `id-field-audit.md`
- `implementation-plan.md`
- `integrations-plan.md`
- `menu-architecture-audit.md`
- `menu-fixes.md`
- `menu-ux-improvements.md`
- `migration-plan.md`
- `navigation-audit.md`
- `navigation-inventory.md`
- `next-phase-audit.md`
- `permission-impact.md`
- `permission-matrix.md`
- `reporting-plan.md`
- `rewrite-plan.md`
- `role-matrix.md`
- `rollout-plan.md`
- `schema-changes.md`
- `security-impact.md`
- `selector-ux-improvements.md`
- `TODO.md`
- `toggl-feature-audit.md`
- `ui-impact-plan.md`
- `workspace-rbac-impact.md`
- `docs/component-inventory.md`
- `docs/ui-audit.md`
- `docs/visual-improvement-plan.md`

The temporary workspace-admin planning files from the workspace management implementation were also merged here and removed:

- `workspace-admin-audit.md`
- `member-management-gap-analysis.md`
- `project-template-plan.md`
- `permission-impact.md`
- `schema-changes.md`
- `ui-flow-plan.md`
- `rollout-plan.md`
- `TODO.md`
