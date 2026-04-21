# Data Model

## New Tables

- `organizations(id, name, slug, created_at)`
- `workspaces(id, organization_id, name, slug, default_currency, timezone, created_at)`
- `organization_members(organization_id, user_id, role, created_at)`
- `workspace_members(workspace_id, user_id, role, created_at)`
- `groups(id, workspace_id, name, description, created_at)`
- `group_members(group_id, user_id, created_at)`
- `project_members(project_id, user_id, role, created_at)`
- `project_groups(project_id, group_id, created_at)`

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
- Existing records are backfilled into the default workspace.
- Existing users are backfilled into the default organization and workspace.
- Project access is granted by direct membership, group membership, workspace admin, or organization admin/owner.

## Indexes

- `workspace_members(user_id, workspace_id)`
- `organization_members(user_id, organization_id)`
- `groups(workspace_id, name)`
- `group_members(user_id, group_id)`
- `project_members(user_id, project_id)`
- `project_groups(group_id, project_id)`
- workspace-prefixed indexes for customers, projects, tags, timesheets, invoices, and webhooks.
