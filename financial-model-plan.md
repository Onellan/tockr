# Financial Model Plan

## Existing model (preserved)
- Money: integer cents throughout.
- Rates: effective-dated, multi-scope (customer/project/activity/task/user).
- Timesheet stores resolved `rate_cents` and `internal_rate_cents` at creation — audit-stable.
- Invoice totals computed from stored rate_cents × duration_seconds / 3600.
- Tax: basis points (e.g. 2000 = 20%).

## Rate recalculation tool

### Purpose
Allow workspace admins to fix timesheets after rate corrections (e.g., wrong rate applied, rate back-dated).

### Scope selectors (UI + store filter)
- Date range (begin/end)
- User (optional)
- Project (optional)
- Exported flag: default exclude exported; override available

### Preview (dry-run)
- Re-run `ResolveRateAt(ctx, workspaceID, userID, customerID, projectID, activityID, taskID, startedAt)` for each matching timesheet.
- Compare current `rate_cents` to resolved rate.
- Show: timesheet ID, date, user, project, current rate, new rate, delta, status.
- No mutations.

### Apply
- Transaction: update `rate_cents`, `internal_rate_cents` for each matching timesheet.
- Audit log: `action=recalculate_rates`, entity=`timesheet`, detail includes count and filter summary.
- Return count of updated rows.

### Protection
- Exported timesheets are excluded by default.
- Audit log entry is created before apply, not after (so even a failed apply is logged).
- Recalculation cannot lower a rate already referenced in a finalized/locked invoice (not currently tracked; deferred if invoice locking is added later).

## Exchange rates

### Storage
- Table: `exchange_rates(workspace_id, from_currency, to_currency, rate_thousandths, effective_from)`
- `rate_thousandths = rate × 1000` (integer, no floats)
- Lookup: `SELECT rate_thousandths FROM exchange_rates WHERE workspace_id=? AND from_currency=? AND to_currency=? AND effective_from <= ? ORDER BY effective_from DESC LIMIT 1`

### Usage in reporting
- Reports page and utilization page show `currency_total` in workspace default currency.
- Conversion: `converted_cents = original_cents * rate_thousandths / 1000`
- If no rate found: show original with currency label, no conversion.
- Rounding: integer truncation (acceptable for display totals).

### What is NOT changed
- Stored timesheet `rate_cents` is never modified by exchange rate logic.
- Invoice currency field on each invoice is the currency at creation time.
- Exchange rates are for display/reporting only unless explicitly noted.

## Invoice financial logic

- Subtotal: `SUM(rate_cents * duration_seconds / 3600)` over matching timesheets.
- Tax: `subtotal * taxBasisPoints / 10000`.
- Total: `subtotal + tax`.
- Richer template shows same numbers with better formatting.
