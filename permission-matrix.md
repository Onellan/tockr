# Permission Matrix

| Capability | Org owner | Org admin | Workspace admin | Analyst | Project manager | Project member | Workspace member |
|---|---:|---:|---:|---:|---:|---:|---:|
| Manage organization | Yes | Yes | No | No | No | No | No |
| Manage workspaces | Yes | Yes | No | No | No | No | No |
| Manage all users | Yes | Yes | Workspace only | No | Project only | No | No |
| Manage workspace members | Yes | Yes | Yes | No | No | No | No |
| Manage groups | Yes | Yes | Yes | No | No | No | No |
| Manage customers/projects/activities/tags | Yes | Yes | Yes | No | Own managed projects only | No | No |
| Manage rates | Yes | Yes | Yes | No | No | No | No |
| Track time | Yes | Yes | Yes | No | Yes | Yes | Yes |
| View own timesheets | Yes | Yes | Yes | No | Yes | Yes | Yes |
| View workspace timesheets | Yes | Yes | Yes | Reports only | Managed projects only | No | No |
| View reports | Yes | Yes | Yes | Yes | Managed projects only | Own data only | Own data only |
| Manage invoices | Yes | Yes | Yes | No | No | No | No |
| Manage webhooks | Yes | Yes | Yes | No | No | No | No |
| Download invoices | Yes | Yes | Yes | No | No | No | No |

## Enforcement Notes

- Backend authorization is authoritative; UI gating is only convenience.
- Workspace/project filters are applied in repository queries.
- Direct URL access must return `403` when the scoped role is insufficient.
- Role changes and membership changes are audit-sensitive operations.
