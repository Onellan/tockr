# Feature Parity Matrix

| Kimai Area | Tockr Decision | Notes |
|---|---|---|
| Login/logout | Keep | Internal username/email + password. |
| Users/roles/permissions | Simplify | Four roles plus permission table. |
| Customers/projects/activities | Keep | Includes billable/visible flags and team hooks. |
| Timesheets | Keep | Active timers, manual entries, breaks, billable/exported state. |
| Tags | Keep | Simple tag CRUD and timesheet tagging. |
| Rates | Simplify | Scoped rates with deterministic precedence. |
| Reports | Keep, simplify | Dashboard and entity summaries. |
| Invoices | Keep basic | Records, metadata, file download, CSV export. |
| API | Simplify | Useful endpoints only, paginated collections. |
| Plugins | Drop | Replaced by webhooks. |
| SAML/LDAP/2FA | Defer/drop | Not part of Pi MVP. |
| Advanced exports | Defer | CSV and simple stored invoice document first. |
| Audit trail | Keep | `audit_log` captures sensitive operations. |

