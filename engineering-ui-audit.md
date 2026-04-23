# Engineering UI Audit

## Target users

- Civil engineers
- Mechanical engineers
- Process engineers

## What was working

- Strong server-rendered structure with low frontend weight.
- Existing dependent selectors already prevented raw-ID workflows.
- Time capture, project dashboards, reports, invoices, rates, templates, and workspace scoping were already present.

## Main friction before this pass

- Dashboard read as a generic admin summary rather than a daily engineering control center.
- Timesheets lacked strong reuse flows for repeated project/task/work-type entry.
- Calendar showed weak project context and did little to support backfill.
- Reports did not foreground billable versus internal work clearly enough.
- Activities were technically correct but semantically unclear for engineering users.
- Projects and project dashboards needed stronger estimate/budget burn visibility.
- Wording across the app leaned generic instead of consulting-engineering operational language.

## High-impact changes implemented

- Navigation grouped around real workflow: Work, Projects / Delivery, Billing / Analysis, Administration.
- UI wording shifted to `Client` and `Work Type` while preserving routes and backend models.
- Dashboard now surfaces:
  - week-to-date tracked time
  - missing time this week
  - recent work reuse
  - favorites
  - project watchlist
  - billing attention
- Timesheets now support query-prefilled entry reuse and show richer billing context in the table.
- Calendar now shows readable project/work-type/task labels and missing-day backfill cues.
- Reports now support billable filtering and stronger engineering/billing framing.
- Project dashboards now show unbilled time/value, billable/internal split, task mix, and contributors.

## Constraints kept intact

- No route model rewrite
- No schema migration
- No SPA conversion
- Existing RBAC and workspace scoping preserved
- Existing Docker/CI flow preserved
