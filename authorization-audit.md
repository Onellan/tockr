# Authorization Audit

## Current Baseline

- The previous model used global roles: `user`, `teamlead`, `admin`, `superadmin`.
- Menu visibility was derived from global permissions.
- Most repository queries listed global rows without workspace filters.
- Timesheets were limited to the current user in the UI/API, but reports and dashboard counters were global.
- Customers, projects, activities, tags, rates, invoices, and webhooks had no workspace ownership boundary.

## Required Corrections

- Add workspace ownership to business records.
- Derive request permissions from organization/workspace/project memberships.
- Apply workspace filters to list and report queries.
- Apply ownership/project scope to timesheet visibility.
- Restrict invoice and webhook API access by workspace role.
- Prevent normal members from seeing unrelated private projects.
- Audit role and membership changes.

## Implementation Notes

- Existing data is backfilled into a default organization and workspace.
- Organization owner/admin and workspace admin can operate across all workspace records.
- Workspace member can track time only against visible/assigned projects.
- Project manager can see managed project timesheets and reports without broad workspace admin powers.
