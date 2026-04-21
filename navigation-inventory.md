# Navigation Inventory

| Location | Label | Target | Permission / role | Current status | Issue | Fix |
|---|---|---:|---|---|---|---|
| Sidebar, brand | Tockr | `/` | logged in | Works | No active semantics | Keep as home link; add accessible shell labels. |
| Sidebar / Work | Dashboard | `/` | logged in | Works | Active state never rendered | Set active state from request path. |
| Sidebar / Work | Timesheets | `/timesheets` | `track_time` intended, logged in currently | Works | Active state missing | Set active state; keep visible for user/teamlead/admin. |
| Sidebar / Manage | Customers | `/customers` | logged in view, `manage_master_data` for create | Works | Create form is visible even to users who cannot submit | Hide create form for users without management permission. |
| Sidebar / Manage | Projects | `/projects` | logged in view, `manage_master_data` for create | Works | Create form is visible even to users who cannot submit | Hide create form for users without management permission. |
| Sidebar / Manage | Activities | `/activities` | logged in view, `manage_master_data` for create | Works | Create form is visible even to users who cannot submit | Hide create form for users without management permission. |
| Sidebar / Manage | Tags | `/tags` | logged in view, `track_time` for create | Works | Active state missing | Set active state. |
| Sidebar / Analyze | Reports | `/reports` | `view_reports` | Route works | Route does not explicitly enforce permission | Add permission check and visibility rule. |
| Sidebar / Analyze | Invoices | `/invoices` | `manage_invoices` | Admin works, normal user sees link then gets 403 | Visible to wrong users | Hide unless permission exists. |
| Sidebar / Admin | Rates | `/rates` | `manage_rates` | Admin works | Group is controlled by broad admin flag | Gate item by exact permission. |
| Sidebar / Admin | Users | `/admin/users` | `manage_users` | Admin route exists | Broad admin gating; route could hang under SQLite single-connection mode | Gate item by exact permission and close user scan before loading roles. |
| Sidebar / Admin | Webhooks | `/webhooks` | `manage_webhooks` | Admin works | Group is controlled by broad admin flag | Gate item by exact permission. |
| Reports tabs | Users | `/reports?group=user` | `view_reports` | Works | Active tab state missing | Add selected tab state. |
| Reports tabs | Customers | `/reports?group=customer` | `view_reports` | Works | Active tab state missing | Add selected tab state. |
| Reports tabs | Projects | `/reports?group=project` | `view_reports` | Works | Active tab state missing | Add selected tab state. |
| Reports tabs | Activities | `/reports?group=activity` | `view_reports` | Works | Active tab state missing | Add selected tab state. |
| Topbar account action | Logout | `POST /logout` | logged in + CSRF | Works | Button label combines name and logout awkwardly | Separate user identity from logout action. |
| Mobile nav | Same sidebar links | same as desktop | same as desktop | Works as stacked links | Crowded, no clear landmark labels | Improve responsive layout, touch targets, focus states. |
| Footer/help | none | none | none | Not present | No broken links | No action. |
