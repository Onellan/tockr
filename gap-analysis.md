# Gap Analysis

## Summary Table

| Feature | Status | Gap |
|---|---|---|
| Favorite edit/delete | Partial | No HTTP routes or UI for edit or delete |
| Task edit/archive | Partial | Edit path exists in store but no HTTP route; no `archived` column |
| Saved report edit/delete | Missing | No update or delete routes or store methods |
| Saved report share links | Missing | `shared` flag exists but no signed-link flow |
| Utilization dashboard | Missing | No utilization query or page |
| Charts | Missing | No chart primitives |
| Rate recalculation | Missing | Rates resolve at creation; no bulk recalc tool |
| Multi-currency | Partial | Currency stored but no exchange rate table or conversion |
| Richer invoice templates | Missing | Current template is a 3-line HTML stub |
| CSV export | Missing | Only HTML invoice download exists |

## Feature Details

### Favorite edit/delete
- Store: needs `UpdateFavorite`, `DeleteFavorite`
- Routes: `POST /favorites/{id}`, `POST /favorites/{id}/delete`
- UI: per-row edit form inline or modal-equivalent; delete with `onsubmit confirm`

### Task edit/archive
- Schema: `ALTER TABLE tasks ADD COLUMN archived INTEGER NOT NULL DEFAULT 0`
- Domain: add `Archived bool` to `Task`
- Store: update `Task` scan and `ListTasks` filter; add `ArchiveTask`
- Routes: `POST /tasks/{id}` (edit), `POST /tasks/{id}/archive`
- UI: per-row edit form + archive button; archived tasks show badge, excluded from dropdowns

### Saved report edit/delete
- Store: `UpdateSavedReport`, `DeleteSavedReport`
- Routes: `POST /reports/saved/{id}`, `POST /reports/saved/{id}/delete`
- UI: edit form in report dropdown row; delete with confirm

### Signed expiring share links
- Schema: `ALTER TABLE saved_reports ADD COLUMN share_token TEXT`, `share_expires_at TEXT`
- Domain: `ShareToken string`, `ShareExpiresAt *time.Time` on `SavedReport`
- Store: `SetReportShareToken`, `FindSharedReport`
- Token format: HMAC-SHA256 over `{report_id}:{expiry_unix}` signed with session secret; URL-safe base64
- Routes: `POST /reports/saved/{id}/share` (generate/regenerate), `GET /reports/share/{token}` (public, no login)
- Shared view: renders report data for the scoped filters without exposing edit controls

### Utilization dashboard
- Store: `UtilizationReport(ctx, access, begin, end *time.Time)` — per-user aggregation of tracked seconds, billable seconds, entry count
- Route: `GET /reports/utilization`
- UI: metric cards per user + inline CSS bar charts for billable vs total

### Charts
- Inline SVG or CSS `<meter>` bars only — no JS library
- Used only on utilization page
- Billable % bar, utilization % bar per user

### Rate recalculation
- Store: `RecalcPreview(ctx, access, filter TimesheetFilter) ([]RecalcRow, error)`, `ApplyRecalc(ctx, access, filter TimesheetFilter) (int, error)`
- `RecalcRow`: timesheet ID, current rate, new rate, delta cents, project, user, date
- Routes: `GET /admin/recalculate` (preview), `POST /admin/recalculate` (apply)
- Only workspace admins can run; excludes exported timesheets by default (configurable)
- Audit log entry on apply

### Multi-currency exchange rates
- Schema: `CREATE TABLE IF NOT EXISTS exchange_rates(id, workspace_id, from_currency, to_currency, rate_thousandths, effective_from, created_at)`
- `rate_thousandths`: integer, rate × 1000, e.g. USD→EUR 0.920 = 920
- Domain: `ExchangeRate` struct
- Store: `UpsertExchangeRate`, `ListExchangeRates`
- Routes: `GET /admin/exchange-rates`, `POST /admin/exchange-rates`
- Utilization and report pages show totals in workspace default currency with conversion where applicable

### Richer invoice templates
- `writeInvoiceFile` takes full context: loads customer, invoice items from store
- Generates HTML with: customer name/email/company, invoice number/date, itemized table (description, hours, rate, total), subtotal, tax, grand total, workspace name
- Backward-compatible: existing invoice records get richer file on re-download (if regenerated)

### CSV export
- Routes: `GET /reports/export?format=csv` (carries same query params as /reports), `GET /timesheets/export?format=csv`
- Response: `Content-Disposition: attachment; filename="report.csv"` / `"timesheets.csv"`
- Uses standard library `encoding/csv`
- Respects same access scoping as the source pages
