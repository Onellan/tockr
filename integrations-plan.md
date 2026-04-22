# Integrations Plan

## Current Surface

- JSON API endpoints for status, lookups, timesheets, invoice download/meta, and webhooks.
- Signed webhook delivery queue.

## Phase 1 Impact

- No heavy integration platform.
- Add API fields for `task_id`, favorites, and saved reports only where useful.
- Emit webhook events for task/favorite/saved report creation in a later pass if integrations need them.

## Deferred

- Scheduled report delivery.
- Native Jira/QuickBooks/Calendar integrations.
- OAuth app marketplace.
- Browser/desktop auto-tracking.

## Security

- API endpoints must reuse scoped authorization.
- Webhook payloads must not leak cross-workspace data.
- Shared report links require signed expiring tokens and are deferred.
