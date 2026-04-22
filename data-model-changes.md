# Data Model Changes

## Phase 1

- `tasks`
  - `id`, `workspace_id`, `project_id`, `name`, `number`, `visible`, `billable`, `estimate_seconds`, `created_at`
- `timesheets.task_id`
  - Optional task reference for more precise reports.
- `favorites`
  - User/workspace-local pinned timer templates with customer/project/activity/task/description/tags.
- `projects.estimate_seconds`
  - Hour budget for project estimate tracking.
- `projects.budget_cents`
  - Fixed-fee or monetary budget target.
- `projects.budget_alert_percent`
  - Threshold for UI alerting.
- `saved_reports`
  - User/workspace saved report configuration: name, group, filters, visibility.

## Deferred

- `rate_versions` for historical rates.
- `user_costs` for profitability.
- `timesheet_approvals` for approval workflows.
- `report_schedules` and share tokens for scheduled/shared reports.

## Migration

- Additive SQLite migration only.
- Existing projects get no estimate/budget by default.
- Existing timesheets have `task_id=NULL`.
- Existing reports are unaffected.
