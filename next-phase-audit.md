# Next Phase Audit

## Phase 1 Feature State

| Feature | Implemented | Gaps / Fragility | Decision |
|---|---|---|---|
| Favorites | Dashboard create/list/start, task-aware timer start, scoped by workspace/user | No edit/delete UI yet | Preserve; extend later only when needed |
| Task tracking | Tasks table, CRUD create/list, task IDs on timesheets, task report grouping | No task edit/archive UI yet | Preserve; add calendar display support |
| Project estimate/budget dashboard | Project fields, dashboard route, threshold alert | Dashboard is read-only | Preserve |
| Saved reports | Save/list/open through report dropdown | No edit/delete/share UI yet | Preserve |
| Task-aware report filters | Filters and saved filter URLs work | No named selectors yet, IDs only | Preserve |

## Navigation And Menus

- Sidebar is grouped and not yet crowded enough for collapsible sections.
- Row overflow is justified only for project rows because project rows have both Dashboard and Membership actions.
- Invoice rows still have one secondary action, so direct Download stays visible.
- Favicon handling is already strong: real root aliases, PNG/ICO assets, manifest, theme color, and cache-busted head links.

## Auth And Account

- Current login is password-only with secure cookie sessions.
- Session middleware can be extended for workspace switching and required TOTP setup enforcement.
- TOTP should be config-driven: `disabled`, `optional`, or `required`.
- Self-service settings do not exist yet and should be added now for profile, password, and TOTP.

## Calendar

- Timesheet list supports scoped date filters.
- First calendar should be read-only week view to avoid drag/drop complexity.
- Calendar must reuse timesheet scope so users do not see cross-workspace or private project data.

## Workspace / RBAC

- Schema supports organizations, workspaces, workspace memberships, project members, and groups.
- Sessions currently choose the first accessible workspace indirectly; explicit switching needs session workspace state.
- Project membership editing exists only as store helpers, not UI.

## Financial Model

- Current rates support customer/project/activity/user scopes and internal amount.
- Historical effective dates and task-level rates are missing.
- User cost tracking needs explicit effective-dated cost rows before profitability reporting.
