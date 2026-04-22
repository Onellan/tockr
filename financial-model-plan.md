# Financial Model Plan

## Implement Now

- Effective-dated billable rates.
- Optional task-scoped rates.
- Effective-dated user cost rates.
- Timesheet creation resolves billing and internal cost rates at the entry start time.

## Calculation Rules

1. Prefer the most specific billable rate valid at `started_at`.
2. Specificity order: activity+user, activity, task+user, task, project+user, project, customer+user, customer, user, workspace default.
3. Internal cost comes from explicit internal rate first, then user cost rate valid at `started_at`.
4. Store resolved cents on the timesheet for audit stability.

## Deferred

- Full profitability dashboards.
- Retroactive recalculation tools.
- Multi-currency conversions.
