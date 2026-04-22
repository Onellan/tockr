# Feature Adoption Plan

| Toggl-inspired feature | What it does | Already present | Decision | Priority | Complexity | Product impact |
|---|---|---:|---|---:|---:|---:|
| Favorites / pinned entries | Save common project/activity/task combinations for one-click reuse | No | Add | P1 | Low | High |
| Faster timer UX | Start from favorite or recent work without typing IDs | Partial | Improve | P1 | Low | High |
| Tasks | Add a task/sub-project layer beneath projects | No | Add | P1 | Medium | High |
| Project estimates | Track estimated hours against actual time | No | Add | P1 | Low | High |
| Fixed-fee budget | Track budget/fixed fee against billable value | No | Add basic | P1 | Low | Medium |
| Project dashboard | Show actual vs estimate/budget progress | No | Add | P1 | Medium | High |
| Saved reports | Save common report filter/group/date configurations | No | Add basic | P1 | Medium | Medium |
| Improved report filters | Filter by date, project, group, user, task | Partial | Improve | P1 | Medium | High |
| Groups/teams | Bulk assignment and reporting | Yes | Keep/improve later | P2 | Medium | Medium |
| Scoped roles | Workspace/project/admin separation | Yes | Keep | P2 | Done | High |
| Approvals | Submit/review time entries | No | Defer | P2 | High | Medium |
| Historical rates | Rate changes across time | No | Defer | P3 | High | Medium |
| Profitability | Revenue minus labor cost | No | Defer | P2/P3 | High | High once data exists |
| Scheduled/shared reports | Email or share reports | No | Defer | P3 | High | Medium |
| Calendar/timeline | Calendar-assisted and auto-tracked entry | No | Defer/reject auto timeline | P3 | High | Medium |
| Native integrations | Jira/QuickBooks/etc. | No | Reject for now | P3 | High | Low for Pi-focused app |

## Phase 1 Implementation Slice

Implement now:

- Favorites and one-click timer starts.
- Tasks under projects.
- Project estimates and fixed-fee budget fields.
- Project dashboard with progress indicators.
- Saved report records and report filters.

This gives the app the biggest Toggl-inspired usability and oversight lift without adding heavy client-side code or operational complexity.
