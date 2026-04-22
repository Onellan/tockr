# Permission and Financial Impact

## Permission Model (unchanged roles)

| Scope | Role | Relevant permissions |
|---|---|---|
| Organization | owner/admin | All workspace admin, recalculation, exchange rates |
| Workspace | admin | All workspace features incl. recalculation, exchange rates, invoice, export |
| Workspace | analyst | Reports, utilization, export |
| Workspace | member | Own timesheets, favorites, own saved reports |
| Project | manager | Project reports, project-scoped timesheets |
| Project | member | Track time on assigned projects |

## Per-Feature Permission Rules

### Favorite edit/delete
- Owner-only: a user can only edit or delete their own favorites.
- Scope check: favorite must belong to `(workspace_id, user_id)` — enforced in store query.
- No admin override needed (admin can always see all data but favorites are personal).

### Task edit/archive
- Requires `PermManageMaster` (workspace admin).
- Task must belong to workspace (workspace_id check in store).
- Archive sets `archived=1` but does NOT delete or change timesheets.

### Saved report edit/delete
- Owner can edit/delete their own saved reports.
- Workspace admin can edit/delete any workspace report.
- Shared reports are visible to all workspace members but only the owner or admin can mutate them.

### Signed expiring share links
- Only the saved report owner or workspace admin can generate a share link.
- Share link access is intentionally unauthenticated (external viewer use case).
- Shared view renders report data scoped to filters stored in the report definition.
- The shared view does NOT expose user emails or admin data.
- Links expire; expired or invalid tokens return 404.

### Utilization dashboard
- Requires `PermViewReports` (workspace admin, analyst, project manager).
- Members without report permission see 403.
- Admin/analyst sees all users. Project manager sees only their managed projects' members.

### Rate recalculation
- Requires `PermManageRates` (workspace admin only).
- Scope: workspace-level; cannot target another workspace.
- Exported timesheets are excluded from recalculation by default; override requires explicit checkbox.
- Recalculation is logged to audit_log.

### Exchange rates
- Requires `PermManageRates` (workspace admin only).
- Exchange rates are workspace-scoped.
- Members see converted totals without needing rate management access.

### Invoice templates
- No new permission required. Invoice generation already requires `PermManageInvoices`.

### CSV exports
- Reports export: requires `PermViewReports`.
- Timesheets export: scoped to the same filter as the timesheets page for the current user.

## Financial Impact

### Rate recalculation
- Modifies `rate_cents` and `internal_rate_cents` on timesheet rows.
- Only applies to non-exported timesheets (default) or exported timesheets if explicitly requested.
- Preview (dry-run) shows affected rows without mutating.
- Audit log records actor, scope, count, and timestamp.
- Recalculation is deterministic: uses the same `ResolveRateAt` logic already in store.

### Exchange rate conversion
- Does NOT modify stored cent values.
- Conversion is applied at display/report time only.
- Exchange rates stored as integer thousandths for precision (no floating point arithmetic in conversion).
- Rate formula: `converted_cents = original_cents * rate_thousandths / 1000`
- Rounding: integer division (truncation toward zero) — acceptable for dashboard display.
- Historical rate lookup: uses `effective_from` to find the rate applicable at the time of the record.

### Invoice totals
- Invoice total generation logic is unchanged.
- The richer HTML template displays the same calculated amounts; formatting only.
- Tax stays as basis-point calculation: `tax = subtotal * taxBasisPoints / 10000`.
