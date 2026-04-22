# TODO

## In Progress
- [ ] All 10 features from feature-audit.md

## Done
- [x] feature-audit.md
- [x] gap-analysis.md
- [x] permission-impact.md
- [x] schema-changes.md
- [x] financial-model-plan.md
- [x] reporting-dashboard-plan.md
- [x] invoice-export-plan.md
- [x] rollout-validation-plan.md

## Feature Implementation Status

### Favorite edit/delete
- [ ] DB: UpdateFavorite, DeleteFavorite
- [ ] Routes: POST /favorites/{id}, POST /favorites/{id}/delete
- [ ] Template: Dashboard edit/delete UI

### Task edit/archive
- [ ] Schema: ALTER TABLE tasks ADD COLUMN archived
- [ ] Domain: Archived bool on Task
- [ ] DB: ArchiveTask, update ListTasks/Task scan
- [ ] Routes: POST /tasks/{id}, POST /tasks/{id}/archive
- [ ] Template: Tasks edit/archive per row

### Saved report edit/delete
- [ ] DB: UpdateSavedReport, DeleteSavedReport
- [ ] Routes: POST /reports/saved/{id}, POST /reports/saved/{id}/delete
- [ ] Template: Reports dropdown with edit/delete

### Signed expiring share links
- [ ] Schema: share_token, share_expires_at on saved_reports
- [ ] DB: SetReportShareToken, FindSharedReport
- [ ] Routes: POST /reports/saved/{id}/share, GET /reports/share/{token}
- [ ] Template: Share button per report + shared view page

### Utilization dashboard
- [ ] DB: UtilizationReport query
- [ ] Route: GET /reports/utilization
- [ ] Template: Utilization page with per-user rows + bar charts

### Rate recalculation
- [ ] DB: RecalcPreview, ApplyRecalc
- [ ] Routes: GET /admin/recalculate, POST /admin/recalculate
- [ ] Template: Recalculate page

### Exchange rates
- [ ] Schema: exchange_rates table
- [ ] DB: UpsertExchangeRate, ListExchangeRates
- [ ] Routes: GET /admin/exchange-rates, POST /admin/exchange-rates
- [ ] Template: ExchangeRates page

### Richer invoice templates
- [ ] DB: InvoiceDetails method
- [ ] Server: update writeInvoiceFile signature
- [ ] Template: Full HTML invoice with customer + line items

### CSV exports
- [ ] Route: GET /reports/export
- [ ] Route: GET /timesheets/export

### Tests
- [ ] Favorite edit/delete test
- [ ] Task archive test
- [ ] Saved report edit/delete test
- [ ] Share link test (valid, expired, invalid)
- [ ] Utilization report test
- [ ] Recalculation test
- [ ] Exchange rate test

## App Lifecycle
- [ ] Build passes
- [ ] Tests pass
- [ ] App relaunched
- [ ] End-to-end validated
- [ ] Committed and pushed to GitHub
- [ ] CI green
# Active Docker Automation TODO

- [x] Audit Docker setup, docs, bootstrap behavior, and CI workflow.
- [x] Generate Docker session secret automatically.
- [x] Generate Docker bootstrap admin password automatically.
- [x] Remove fixed Docker admin password from Compose defaults.
- [x] Rewrite canonical Docker setup documentation.
- [x] Run local Go and Docker validation.
- [ ] Push changes to GitHub.
- [ ] Iterate GitHub Actions until CI succeeds.
- [ ] Pull the GHCR image produced by GitHub.
- [ ] Validate install from the published image.
