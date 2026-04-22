# Data Model

## New Tables

- `organizations(id, name, slug, created_at)`
- `workspaces(id, organization_id, name, slug, description, default_currency, timezone, archived, created_at)`
- `organization_members(organization_id, user_id, role, created_at)`
- `workspace_members(workspace_id, user_id, role, created_at)`
- `groups(id, workspace_id, name, description, created_at)`
- `group_members(group_id, user_id, created_at)`
- `project_members(project_id, user_id, role, created_at)`
- `project_groups(project_id, group_id, created_at)`
- `project_templates(id, workspace_id, name, description, project defaults, archived, created_at)`
- `project_template_tasks(id, template_id, name, number, visible, billable, estimate_seconds)`
- `project_template_activities(id, template_id, name, number, visible, billable)`

## Scoped Columns

Workspace-owned records get `workspace_id`:

- customers
- projects
- activities
- tags
- rates
- timesheets
- invoices
- webhook_endpoints

Users get `organization_id` so each user has one owning organization.

Projects get `private` so assignment can restrict visibility.

## Integrity Rules

- Workspace IDs are required for new business records.
- Workspaces are organization-scoped and can be archived without deleting historical data.
- Existing records are backfilled into the default workspace.
- Existing users are backfilled into the default organization and workspace.
- Project access is granted by direct membership, group membership, workspace admin, or organization admin/owner.
- Project templates copy project defaults, tasks, and activities, but never copy live time, invoices, rates, memberships, favorites, or audit data.

## Indexes

- `workspace_members(user_id, workspace_id)`
- `organization_members(user_id, organization_id)`
- `groups(workspace_id, name)`
- `group_members(user_id, group_id)`
- `project_members(user_id, project_id)`
- `project_groups(group_id, project_id)`
- `project_templates(workspace_id, name)`
- workspace-prefixed indexes for customers, projects, tags, timesheets, invoices, and webhooks.
