# Hierarchy UI Plan

## Navigation

- Show organization/workspace context in the topbar.
- Keep normal member navigation focused on Dashboard, Timesheets, assigned Manage lists, and Reports where allowed.
- Show Members and Groups only to workspace admins and organization admins/owners.
- Keep Billing/Admin actions hidden unless scoped authorization allows them.

## Admin Screens

- Users: show organization role and workspace role, not only legacy roles.
- Groups: workspace-local CRUD with member assignment.
- Projects: show visibility, project members, and project managers.

## Member Experience

- Regular members should not need to understand the full hierarchy.
- They see only allowed workspace data and assigned/public projects.
- Forms should not offer actions that backend authorization will reject.

## Lightweight Implementation

- Server-rendered forms and tables only.
- No client-side role engine.
- No dropdown-heavy hierarchy editor in the first pass.
- Workspace switcher is documented for follow-up; this implementation selects the first accessible workspace.
