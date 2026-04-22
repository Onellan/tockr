# Reporting and Dashboard Plan

## Existing reports (preserved)
- Reports page: groupBy (user/customer/project/activity/task/group) + filter + tabular output.
- Saved reports: create, list, open (apply filter state).
- Project dashboard: estimate %, budget %, tracked time, billable value.
- Main dashboard: 4 summary cards + timer + favorites.

## New: Utilization dashboard (`/reports/utilization`)

### Purpose
Give workspace admins and analysts a fast view of workload distribution and billable efficiency.

### Metrics per user (in date range)
- Total tracked seconds
- Billable tracked seconds
- Billable %: `billable_seconds / total_seconds × 100`
- Entry count
- Revenue (billable rate × duration)

### Date filter
- Default: current month (first day to today)
- User-selectable begin/end dates

### Scope
- Workspace admin/analyst: all users in workspace
- Project manager: users from managed projects only
- Regular member: own data only

### Display
- Metric card grid showing workspace-level totals
- Per-user table with inline CSS progress bar for billable %
- Simple, no JS

### Chart approach (lightweight)
- CSS `<span>` with inline `width: X%` and background color.
- Rendered server-side. No JS library. No SVG.
- Example: `<div class="bar-track"><span class="bar-fill billable" style="width: 72%"></span></div>`

## Saved report improvements

### Edit saved report
- Inline form in reports page sidebar/dropdown.
- Can update: name, shared flag.
- Filter state is shown but not editable (user must re-run and save new).
- Route: `POST /reports/saved/{id}`

### Delete saved report
- Delete button with `onsubmit confirm` in dropdown.
- Route: `POST /reports/saved/{id}/delete`

### Share link
- Share button per report for owner/admin.
- Generates HMAC-signed token: `hex(random_32_bytes)` stored in DB; HMAC is signed by server for validation, not stored.
- Actually: token is random 32 bytes stored in DB; link validity checked by DB lookup + expiry check. HMAC signing is applied to the URL param for additional tamper protection.
- Link format: `/reports/share/{token}`
- Expiry: configurable (default 7 days, options: 1d, 7d, 30d, 90d).
- Shared view renders report output without logged-in context.
- Revoking: regenerate or clear token (POST /reports/saved/{id}/share with `action=revoke`).

## Export

### CSV report export
- Route: `GET /reports/export?format=csv&group=...&begin=...&end=...&...`
- Same filter params as `/reports`
- Headers: group name, entries, duration_seconds, revenue_cents
- Filename: `report-{group}-{date}.csv`

### CSV timesheet export
- Route: `GET /timesheets/export?format=csv&...`
- Same filter params as `/timesheets`
- Headers: id, date, start, end, duration_seconds, user, customer, project, activity, task, billable, exported, description, rate_cents
- Filename: `timesheets-{date}.csv`

## Nav additions
- "Utilization" under Analyze group (requires `PermViewReports`)
- Export links added inline on Reports and Timesheets pages (no new nav item needed)
