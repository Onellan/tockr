# Schema

SQLite is the first database. PostgreSQL can be added later by implementing the same repository interfaces.

## Storage Rules

- SQLite pragmas: `journal_mode=WAL`, `foreign_keys=ON`, `busy_timeout=5000`.
- Timestamps are UTC RFC3339 text.
- Money is integer cents.
- Booleans are `0` or `1`.
- Long-running imports happen outside request paths.

## Core Tables

- `organizations`, `workspaces`
- `users`, `roles`, `user_roles`, `organization_members`, `workspace_members`, `sessions`
- `groups`, `group_members`, `project_members`, `project_groups`
- legacy compatibility: `teams`, `team_members`, `customer_teams`, `project_teams`, `activity_teams`
- `customers`, `projects`, `activities`, `tasks`
- `favorites`, `saved_reports`
- `rates`
- `timesheets`, `tags`, `timesheet_tags`
- `invoices`, `invoice_items`, `invoice_meta`
- `webhook_endpoints`, `webhook_deliveries`
- `audit_log`, `settings`, `schema_migrations`

Workspace-owned records include `workspace_id`. Users include `organization_id`. Private projects use `projects.private` plus `project_members`/`project_groups` for scoped visibility. Projects include lightweight estimate and budget fields. Tasks sit below projects and can be attached to timesheets, favorites, and reports.

## Key Indexes

- `timesheets(user_id, started_at DESC)`
- `timesheets(workspace_id, started_at DESC)`
- `timesheets(project_id, started_at)`
- `timesheets(activity_id, started_at)`
- `timesheets(task_id, started_at)`
- `timesheets(exported, billable)`
- `projects(customer_id, visible)`
- `projects(workspace_id, visible)`
- `activities(project_id, visible)`
- `tasks(project_id, visible)`
- `favorites(user_id, workspace_id)`
- `saved_reports(user_id, workspace_id)`
- `invoices(customer_id, created_at DESC)`
- `workspace_members(user_id, workspace_id)`
- `project_members(user_id, project_id)`
- `group_members(user_id, group_id)`
- unique `users.email`, `roles.name`, `tags(workspace_id, name)`, `settings.name`
