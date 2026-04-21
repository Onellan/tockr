# Migration Plan

## Hierarchy Backfill

- Create one default organization named `Default Organization`.
- Create one default workspace named `Default Workspace`.
- Assign all existing users to the default organization.
- Map legacy `superadmin` users to organization owner and workspace admin.
- Map legacy `admin` users to organization admin and workspace admin.
- Map legacy `teamlead` users to workspace analyst/member, then promote to project manager only when legacy team/project evidence exists.
- Map legacy `user` users to workspace member.
- Assign all existing customers, projects, activities, tags, rates, timesheets, invoices, and webhooks to the default workspace.
- Existing projects remain public unless a future migration can infer restricted team visibility.
- Existing `teams` data can be migrated to `groups`; `team_members` becomes `group_members`.

## Integrity Checks

- Every user has `organization_id`.
- Every user has at least one `organization_members` row.
- Every active user has at least one `workspace_members` row.
- Every customer/project/activity/tag/rate/timesheet/invoice/webhook row has `workspace_id`.
- Every timesheet project belongs to the same workspace as the timesheet.

## Source

The migration utility reads an existing Kimai database via a legacy DSN. The source database is read-only from Tockr's point of view.

## Mapping

- `kimai2_users` -> `users`, `user_roles`
- `kimai2_customers` -> `customers`
- `kimai2_projects` -> `projects`
- `kimai2_activities` -> `activities`
- `kimai2_timesheet` -> `timesheets`
- `kimai2_tags`, `kimai2_timesheet_tags` -> `tags`, `timesheet_tags`
- `kimai2_*_rates` -> `rates`
- `kimai2_invoices`, `kimai2_invoices_meta` -> `invoices`, `invoice_meta`
- team join tables -> team assignment tables

## Transformations

- PHP serialized roles are parsed into role names.
- Float money/rates are rounded to integer cents.
- Legacy unsupported fields are stored in `legacy_json` or metadata rows where useful.
- Password hashes are preserved when compatible; otherwise users require password reset.

## Rollback

Rollback is restore-from-backup. The migration never mutates the source Kimai database.
