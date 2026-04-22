# Schema Changes

## Users

- `totp_secret`
- `totp_enabled`
- `totp_recovery_hashes`

## Sessions

- `workspace_id`

## Rates

- `task_id`
- `effective_from`
- `effective_to`

## User Cost Rates

New table: `user_cost_rates`

- `id`
- `workspace_id`
- `user_id`
- `amount_cents`
- `effective_from`
- `effective_to`
- `created_at`

Indexes:

- `rates(workspace_id, task_id, effective_from)`
- `user_cost_rates(workspace_id, user_id, effective_from)`

## Migration

- Additive `ALTER TABLE` only.
- Existing rate rows get `effective_from = 1970-01-01T00:00:00Z`.
- Existing sessions get workspace `1` or the user's first available workspace.
