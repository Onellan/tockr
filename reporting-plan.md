# Reporting Plan

## Phase 1

- Keep report output simple and table-based.
- Add filters for `begin`, `end`, `project_id`, `group_id`, `user_id`, and `task_id`.
- Add task grouping.
- Add saved report definitions for repeatable filters.
- Add project dashboard metrics:
  - tracked seconds
  - billable value
  - estimate progress
  - budget progress
  - alert threshold state

## Deferred

- Scheduled report delivery.
- Shareable report links.
- Profitability and utilization dashboards.
- Charting beyond lightweight progress bars.

## Design Notes

Reports must stay fast on SQLite. Avoid materialized aggregates until data volume proves it is needed.
