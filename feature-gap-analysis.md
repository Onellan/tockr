# Feature Gap Analysis

| Area | Current Tockr | Gap | Recommendation |
|---|---|---|---|
| Timer UX | Start/stop with IDs | Repeated work requires retyping IDs | Add favorites and recent-entry quick starts. |
| Manual entry | Basic form | No task field, no recent context | Add optional task and keep form compact. |
| Project structure | Customer -> project -> activity | No task/sub-project layer | Add tasks under projects for precision. |
| Project oversight | Lists only | No estimate/budget progress view | Add project dashboard and estimate/budget fields. |
| Reporting | Basic group summaries | No saved filters/date ranges/task grouping | Add filters, task grouping, saved reports. |
| Rates/billing | Scoped rates and invoices | No historical rates or labor costs | Defer deeper profitability until Phase 2/3. |
| Team/admin | Scoped hierarchy exists | Membership editor is minimal | Keep groups, defer richer bulk UI. |
| Integrations | API + webhooks | Event list not complete | Add events for new domain actions later. |
| Approvals | None | No review/lock workflow | Defer until teams need governance. |

## Do Not Change

- Keep HTML-first server rendering.
- Keep SQLite-first deployment.
- Keep the current Kimai-inspired information architecture.
- Keep workflows lightweight for small teams and Raspberry Pi deployments.
