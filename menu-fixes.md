# Menu Fixes

## Required Fixes

- Hide `Invoices` unless the user has `manage_invoices`.
- Gate admin menu items by exact permissions instead of a broad admin boolean.
- Mark the current sidebar item with `aria-current="page"` and an active class.
- Mark the selected reports tab with `aria-current="page"`.
- Add explicit `view_reports` enforcement to `/reports`.
- Hide create forms on master-data pages from users who can view but cannot create.
- Make topbar account/logout controls clearer and more accessible.
- Fix `/admin/users` SQLite single-connection hang by loading roles after the user rows cursor is closed.

## Verification Targets

- Admin can open every visible sidebar item with `200`.
- Normal user does not see invoice/admin links.
- Normal user receives `403` for direct admin or invoice URLs.
- Active state appears for `/`, `/timesheets`, entity lists, reports, invoices, and admin pages.
- Mobile layout keeps all visible links reachable without horizontal page overflow.

## Completed

- All required fixes above were implemented.
- `go test ./...` passes.
- Docker rebuild succeeded and `/healthz` returns `200` on port `8029`.
- Browser validation covered admin navigation, normal-user visibility, direct forbidden routes, and mobile layout.
