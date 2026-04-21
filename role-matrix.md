# Role Matrix

| Scope | Role | Intended user | Main capabilities |
|---|---|---|---|
| Organization | owner | Account owner | Full organization control, workspace management, user and role assignment, audit/security oversight. |
| Organization | admin | Senior administrator | Organization-wide administration and reporting, workspace/member management. |
| Workspace | admin | Operations/admin lead | Manage workspace settings, members, groups, projects, customers, activities, tags, rates, invoices, webhooks, and workspace reports. |
| Workspace | analyst | Reporting user | View workspace reports and read-only operational data. |
| Workspace | member | Regular user | Track time, view assigned/public projects, manage own timesheets. |
| Project | manager | Project lead | Manage assigned project membership and view project-scoped reporting/timesheets. |
| Project | member | Contributor | Track time and view project-scoped data. |

## Legacy Role Mapping

| Legacy role | New mapping |
|---|---|
| `superadmin` | Organization owner + workspace admin in default workspace. |
| `admin` | Organization admin + workspace admin in default workspace. |
| `teamlead` | Workspace analyst + project manager when imported project/team data can prove scope; otherwise workspace member. |
| `user` | Workspace member. |
