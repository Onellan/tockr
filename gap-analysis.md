# Gap Analysis

## Must Implement Now

- Account/profile page with password change and optional TOTP setup.
- TOTP config and validation flow.
- Read-only weekly calendar.
- Session-scoped workspace switcher.
- Project membership editing UI for users and groups.
- Historical rate effective dates, task-scoped rates, and user cost rates.
- Project row overflow menu.

## Should Defer

- Collapsible sidebar: the menu is still small and visible navigation is faster.
- Editable calendar drag/drop: adds complexity and accidental-edit risk.
- Full profitability reporting: wait until historical rates and user costs have production data.
- Saved report edit/delete/share UI: useful but not part of this slice.

## Compatibility

- All schema changes are additive.
- Existing timesheets keep stored billing/internal rates.
- Existing rates are backfilled to `1970-01-01T00:00:00Z` effective start.
- Existing sessions can keep working because session workspace defaults to the first accessible workspace.
