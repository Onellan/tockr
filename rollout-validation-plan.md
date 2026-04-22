# Rollout and Validation Plan

## Local development validation

### Starting the app
```bash
go build -o /tmp/tockr ./cmd/app
TOCKR_SESSION_SECRET=dev-secret-change-me \
TOCKR_ADMIN_PASSWORD=admin12345 \
/tmp/tockr
```
App starts on port 8080 by default. `/healthz` returns `{"status":"ok"}`.

### End-to-end validation checklist

#### Favorites
- [ ] Dashboard loads with existing favorites
- [ ] Create new favorite → appears in list
- [ ] Edit favorite (rename, change description) → saved correctly
- [ ] Delete favorite → removed, no 500, confirm dialog fired

#### Tasks
- [ ] Tasks page shows archived badge on archived tasks
- [ ] Create task → appears in list
- [ ] Edit task (name, estimate) → saved correctly
- [ ] Archive task → task gets `archived=1`, disappears from dropdowns, visible in task list with badge
- [ ] Timesheet create still works after task archived (task visible in existing timesheet, not in new dropdown)

#### Saved reports
- [ ] Create saved report → appears in dropdown
- [ ] Edit saved report name and shared flag → saved
- [ ] Delete saved report → removed from dropdown
- [ ] Generate share link → link appears with expiry
- [ ] Share link GET → report renders without login
- [ ] Expired share link → 404 returned
- [ ] Invalid token → 404 returned

#### Utilization dashboard
- [ ] /reports/utilization loads with date filter
- [ ] Shows per-user rows with correct hours
- [ ] Billable % bar renders correctly
- [ ] Admin sees all users; member sees only self

#### Rate recalculation
- [ ] /admin/recalculate preview shows affected timesheets
- [ ] Apply recalculates correctly
- [ ] Exported timesheets excluded by default
- [ ] Audit log entry written on apply

#### Exchange rates
- [ ] /admin/exchange-rates page loads
- [ ] Add exchange rate → appears in list
- [ ] Reports utilization shows converted total where rate exists

#### Invoice template
- [ ] Create new invoice → HTML file contains customer name and line items
- [ ] Download invoice → readable HTML with table of entries

#### CSV exports
- [ ] GET /reports/export returns CSV with correct headers and rows
- [ ] GET /timesheets/export returns CSV with correct headers and rows
- [ ] Scope is respected (member only sees own data)

## CI validation

### Pipeline stages
1. `validate`: `go test ./...` + `go build -trimpath -ldflags="-s -w" -o /tmp/tockr ./cmd/app`
2. `container-smoke`: Docker build + health check on `/healthz`
3. `docker-image`: Multi-platform build + GHCR push (main branch / tags only)

### CI success criteria
- All Go tests pass
- Binary compiles without errors
- Container starts and responds healthy
- GHCR push succeeds (on main branch push)

### Merge strategy
- Commit all changes with descriptive message
- Push to main branch (this repo's workflow)
- Confirm GitHub Actions run completes all stages green

## Regression checks
- Existing tests must still pass (no behavior change to tested paths)
- Login/logout/CSRF still works
- Timesheet create/start/stop still works
- Invoice create still works
- Report filters still work
- Project dashboard still works
