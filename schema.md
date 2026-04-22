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
- `project_templates`, `project_template_tasks`, `project_template_activities`
- `favorites`, `saved_reports`
- `rates`, `user_cost_rates`
- `timesheets`, `tags`, `timesheet_tags`
- `invoices`, `invoice_items`, `invoice_meta`
- `webhook_endpoints`, `webhook_deliveries`
- `audit_log`, `settings`, `schema_migrations`

Workspace-owned records include `workspace_id`. Workspaces include description and archived state. Users include `organization_id`, optional TOTP state, and hashed recovery codes. Sessions include the selected `workspace_id`. Private projects use `projects.private` plus `project_members`/`project_groups` for scoped visibility. Projects include lightweight estimate and budget fields. Tasks sit below projects and can be attached to timesheets, favorites, reports, and task-scoped rates. Project templates are workspace-scoped and copy safe setup defaults, tasks, and activities only.

Rates support `effective_from`/`effective_to`, optional task scope, and stored `internal_amount_cents`. User cost rates are separately effective-dated in `user_cost_rates`; timesheets store the resolved billing and internal cents at creation time for audit stability.

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
- `rates(workspace_id, task_id, effective_from DESC)`
- `user_cost_rates(workspace_id, user_id, effective_from DESC)`
- `invoices(customer_id, created_at DESC)`
- `workspace_members(user_id, workspace_id)`
- `project_members(user_id, project_id)`
- `group_members(user_id, group_id)`
- `project_templates(workspace_id, name)`
- unique `users.email`, `roles.name`, `tags(workspace_id, name)`, `settings.name`
