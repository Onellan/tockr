# Schema Changes

All changes are additive. Existing data and existing code paths are preserved.

## 1. Add `archived` to tasks

```sql
ALTER TABLE tasks ADD COLUMN archived INTEGER NOT NULL DEFAULT 0;
```

- Default `0` — all existing tasks are treated as active.
- Archived tasks are excluded from selector dropdowns (`visible=1 AND archived=0`).
- Archived tasks remain accessible in timesheet history and reports.

## 2. Add share link columns to saved_reports

```sql
ALTER TABLE saved_reports ADD COLUMN share_token TEXT;
ALTER TABLE saved_reports ADD COLUMN share_expires_at TEXT;
```

- `share_token`: opaque hex string (32 bytes random), NULL when not shared via link.
- `share_expires_at`: UTC RFC3339 expiry, NULL until a link is generated.
- Existing rows default NULL — no links exist until explicitly created.

## 3. Add exchange_rates table

```sql
CREATE TABLE IF NOT EXISTS exchange_rates (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  from_currency TEXT NOT NULL,
  to_currency TEXT NOT NULL,
  rate_thousandths INTEGER NOT NULL,
  effective_from TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_exchange_rates_workspace ON exchange_rates(workspace_id, from_currency, to_currency, effective_from DESC);
```

- `rate_thousandths`: rate × 1000 (integer). USD→EUR 0.92 → 920.
- Multiple rates per pair allowed (effective-dated).
- Newest `effective_from ≤ target_date` is used for conversion.

## Migration strategy

The `Migrate` function runs `CREATE TABLE IF NOT EXISTS` statements idempotently.
For column additions, an `ensureColumns` helper is called from `Migrate` after the schema DDL.

```go
func (s *Store) ensureColumns(ctx context.Context) error {
    alterations := []struct{ table, column, def string }{
        {"tasks", "archived", "INTEGER NOT NULL DEFAULT 0"},
        {"saved_reports", "share_token", "TEXT"},
        {"saved_reports", "share_expires_at", "TEXT"},
    }
    for _, a := range alterations {
        _, err := s.db.ExecContext(ctx, fmt.Sprintf(
            "ALTER TABLE %s ADD COLUMN %s %s", a.table, a.column, a.def))
        if err != nil && !strings.Contains(err.Error(), "duplicate column") {
            return err
        }
    }
    return nil
}
```

## Indexes added

```sql
CREATE INDEX IF NOT EXISTS idx_tasks_project_archived ON tasks(project_id, archived, name);
CREATE INDEX IF NOT EXISTS idx_exchange_rates_workspace ON exchange_rates(workspace_id, from_currency, to_currency, effective_from DESC);
```

## No breaking changes

- All new columns have safe defaults.
- All existing queries that do not reference new columns continue to work.
- `ListTasks` gains optional `archived=0` filter for active-only view (default).
- `ListSavedReports` gains new columns in scan but they are NULLable.
