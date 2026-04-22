# Workspace RBAC Impact

## Current Foundation

- Organization owner/admin can administer all workspaces.
- Workspace admin administers one workspace.
- Project manager/member scopes project access.
- Group-project assignments grant project membership-like access.

## New Behavior

- Sessions carry the selected workspace.
- The topbar shows a workspace switcher when more than one workspace is available.
- Project membership editor allows user role assignment and group assignment inside the current workspace.
- Access checks continue to happen server-side in store queries and route middleware.

## Deferred

- Workspace creation UI.
- Organization-level workspace administration screens.
