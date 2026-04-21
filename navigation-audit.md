# Navigation Audit

## Scope

The current navigation is generated in `web/templates/templates.go`, permission checks are enforced in `internal/platform/http/server.go`, and role permissions live in `internal/auth/permissions.go`.

## Broken Or Risky Items

1. `Invoices` appears for normal users, but `/invoices` requires `manage_invoices` and returns `403`.
2. Sidebar active state never appears because `renderNav` is always called with an empty active path.
3. `Reports` is visible to roles with `view_reports`, but the route did not explicitly enforce that permission.
4. Customers, projects, and activities show create forms to users who can view those lists but cannot submit the form.
5. Admin navigation is controlled by a broad `admin` flag instead of per-item permissions.
6. Reports tabs do not show which report group is selected.
7. Topbar account action combines user identity and logout into one label, which is less clear for screen readers and keyboard users.
8. Mobile navigation works as stacked links, but grouping, focus, and touch-target polish need improvement.
9. `/admin/users` can hang under SQLite single-connection mode because user rows and role lookups are queried at the same time.

## UI / UX Critique

- The Kimai-like left rail is a good structure and should stay.
- Group labels are useful, but the admin and billing visibility rules need to match actual permissions.
- Navigation has no selected-page feedback, which makes dense admin screens feel unanchored.
- Topbar hierarchy is thin: the current page is repeated, while account controls are too compressed.
- Keyboard focus styles rely mostly on browser defaults; links and buttons need consistent focus rings.
- Mobile navigation is usable but not refined: it needs clearer group spacing and larger touch targets.

## Proposed Navigation Structure

Keep the current information architecture and tighten permission-aware grouping:

- Work: Dashboard, Timesheets
- Manage: Customers, Projects, Activities, Tags
- Analyze: Reports, Invoices when invoice permission exists
- Admin: Rates, Users, Webhooks when each exact permission exists

Normal users should see only the destinations they can open successfully. Admins and superadmins should see the full set.

## Implementation Plan

1. Add permission-aware navigation items and current-path active state.
2. Hide forms whose submit routes require permissions the user lacks.
3. Enforce `view_reports` on the reports route.
4. Add selected report tabs and stronger focus/hover/active styling.
5. Improve topbar account area and responsive navigation spacing.
6. Add route and visibility regression tests for admin and normal users.
7. Validate visible menu links through Docker and browser automation.

## Validation Notes

- Admin browser pass opened Dashboard, Timesheets, Customers, Projects, Activities, Tags, Reports, Invoices, Rates, Users, and Webhooks.
- Normal-user browser pass showed only Dashboard, Timesheets, Customers, Projects, Activities, Tags, and Reports.
- Direct normal-user access to `/invoices` and `/admin/users` returned `403` as expected.
- Mobile viewport at `390x844` kept the navigation reachable and showed no console warnings.
