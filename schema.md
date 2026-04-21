# Schema

SQLite is the first database. PostgreSQL can be added later by implementing the same repository interfaces.

## Storage Rules

- SQLite pragmas: `journal_mode=WAL`, `foreign_keys=ON`, `busy_timeout=5000`.
- Timestamps are UTC RFC3339 text.
- Money is integer cents.
- Booleans are `0` or `1`.
- Long-running imports happen outside request paths.

## Core Tables

- `users`, `roles`, `user_roles`, `role_permissions`, `sessions`
- `teams`, `team_members`, `customer_teams`, `project_teams`, `activity_teams`
- `customers`, `projects`, `activities`
- `rates`
- `timesheets`, `tags`, `timesheet_tags`
- `invoices`, `invoice_items`, `invoice_meta`
- `webhook_endpoints`, `webhook_deliveries`
- `audit_log`, `settings`, `schema_migrations`

## Key Indexes

- `timesheets(user_id, started_at DESC)`
- `timesheets(project_id, started_at)`
- `timesheets(activity_id, started_at)`
- `timesheets(exported, billable)`
- `projects(customer_id, visible)`
- `activities(project_id, visible)`
- `invoices(customer_id, created_at DESC)`
- unique `users.email`, `roles.name`, `tags.name`, `settings.name`

