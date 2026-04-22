# Toggl-Inspired Feature Audit

## Current Tockr App

Tockr already has a lightweight server-rendered foundation:

- Organization/workspace/group/project scoped permissions.
- Customers, projects, activities, tags, rates, timesheets, invoices, reports, users, groups, and webhooks.
- Start/stop timer and manual time entry.
- SQLite-first deployment with Docker/systemd support.
- Basic reports grouped by user, customer, project, activity, and group.
- Basic invoice creation/download and invoice metadata API.

## Toggl-Inspired Product Patterns

Public Toggl Track material emphasizes several product patterns: one-click timers, favorites, tasks beneath projects, project estimates and alerts, fixed-fee budgets, historical rates, profitability reporting, saved/scheduled/shared reports, teams/groups, workspace access levels, required fields, locked time entries, approvals, API/webhooks, and calendar/timeline-assisted tracking.

Sources reviewed:

- https://toggl.com/track/features/
- https://toggl.com/track/time-reporting/
- https://support.toggl.com/project-time-estimates

## Classification

| Feature pattern | Classification | Reason |
|---|---|---|
| Favorites / pinned entries | Must-have | Big speed win, tiny implementation footprint. |
| One-click / recent timer starts | Must-have | Fits current timer UX and reduces repeated data entry. |
| Tasks under projects | Must-have | Adds useful reporting precision without enterprise weight. |
| Project estimates | Must-have | High operational value and simple schema. |
| Budget / fixed-fee tracking | High value | Useful for invoicing/profitability, can start simple. |
| Saved reports | High value | Improves repeat use without scheduled delivery complexity. |
| Project dashboard | High value | Makes estimates/budgets visible where decisions happen. |
| Teams/groups access | Already present | Keep improving under scoped hierarchy. |
| Historical rates | Defer | Valuable but needs careful billing semantics. |
| Profitability reporting | Defer after budget/rate data matures | Needs labor costs and historical rates to be trustworthy. |
| Approvals / locked entries | Defer | Useful for larger teams, but adds process overhead. |
| Scheduled/shared reports | Defer | Needs email/link security and expiry design. |
| Calendar/timeline tracking | Defer/reject for MVP | Calendar is useful; automated timeline is too heavy/privacy-sensitive. |
| Native external integrations | Defer | Keep API/webhooks lightweight first. |
