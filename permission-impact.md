# Permission Impact

## Phase 1 Rules

- Favorites are private to the current user within a workspace.
- Tasks inherit project visibility and workspace scope.
- Workspace admins can create/edit tasks and project estimate/budget fields.
- Project managers can use tasks and see project dashboard data for managed projects.
- Members can track time only against public or assigned projects/tasks.
- Saved reports are private by default; workspace admins can later create shared workspace reports.
- Project dashboards respect the same project visibility rules as project lists.

## Backend Enforcement

- UI hiding is not trusted.
- Timer and manual entry validate project access before writing.
- Report filters are applied through scoped repository queries.
- API endpoints reuse the same access context.

## Audit Events

Add audit entries for:

- favorite created
- task created
- saved report created
- project estimate/budget changes
