# Feature Audit

## Favorites
- **Exists**: create favorite, list favorites, start timer from favorite.
- **Missing**: edit favorite (rename, change project/activity/task/description/tags), delete favorite.
- **Fragile**: favorites display name-only in the dashboard; no indication of project/activity. No inline destroy confirm.
- **Action**: add edit form per row + delete button with confirm.

## Tasks
- **Exists**: create task (UpsertTask with INSERT path), list tasks, task-scoped timesheets and reports.
- **Missing**: edit task in UI (UPDATE path is in store but no HTTP route exists that passes an ID), archive task (no `archived` column — only `visible`).
- **Fragile**: "visible=false" currently conflates hidden and retired. Historical timesheets reference task IDs; hard-deleting or toggling visible would be confusing.
- **Action**: add `archived` column to tasks table; add edit form per row; add archive action per row.

## Saved Reports
- **Exists**: create saved report, list saved reports by owner + shared, open saved report via dropdown.
- **Missing**: edit saved report (name, filters, group, shared flag), delete saved report, sharing via signed expiring link.
- **Fragile**: saved reports have `shared` flag but sharing only means "visible to all workspace members" — no link-based access for external viewers.
- **Action**: add edit/delete per report; add share-link generation with HMAC token + expiry; add public shared-report view route.

## Dashboards
- **Exists**: main dashboard (4 metrics + timer + favorites); project dashboard (4 metrics + alert).
- **Missing**: utilization/workload dashboard (per-user hours vs capacity, billable %, over/under).
- **Fragile**: dashboard cards are read-only summaries. No date filtering on main dashboard.
- **Action**: add /reports/utilization page with per-user breakdown.

## Charts
- **Exists**: none.
- **Missing**: inline charts on utilization dashboard to make scanning faster.
- **Action**: add CSS/SVG bar charts on utilization page only (no JS dependency).

## Rates and Rate History
- **Exists**: effective-dated billable rates and user cost rates, full scope (customer/project/activity/task/user), resolved at timesheet creation.
- **Missing**: retroactive recalculation tool (bulk re-resolve rates for timesheet range, dry-run + apply).
- **Fragile**: no admin UI to identify and fix timesheets with stale or zero rates.
- **Action**: add /admin/recalculate page with scope filters, preview table, and apply action.

## Currency
- **Exists**: `default_currency` on workspace; `currency` on customer and invoice.
- **Missing**: exchange rate table, manual rate input, conversion in reporting totals, cross-currency report display.
- **Fragile**: all money is stored as integer cents but no currency label is shown in reports.
- **Action**: add `exchange_rates` table; add exchange rate admin; add workspace-currency-converted totals in utilization/reports.

## Invoice Templating
- **Exists**: invoice record created from unexported billable timesheets; HTML file written as stub (`<title>INV-…</title><h1>…</h1><p>Total: …</p>`).
- **Missing**: customer info, itemized line table, subtotal/tax/total, notes, payment terms.
- **Fragile**: the invoice HTML file is not readable by customers. No invoice items are referenced in the download.
- **Action**: rewrite `writeInvoiceFile` to produce a proper HTML invoice with all data.

## Export
- **Exists**: HTML invoice file download via `/api/invoices/{id}/download`.
- **Missing**: CSV export for reports, CSV export for timesheets.
- **Fragile**: no export option on report or timesheet pages.
- **Action**: add `GET /reports/export?format=csv` and `GET /timesheets/export?format=csv`.

## CI/CD
- **Exists**: GitHub Actions pipeline: Go test + build → container smoke test → GHCR publish. Runs on push/PR to main and on version tags.
- **Missing**: nothing functionally missing; CI is complete and working.
- **Note**: CI path-ignore skips markdown and docs/ changes. Code changes always trigger full pipeline.
