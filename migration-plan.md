# Migration Plan

## Source

The migration utility reads an existing Kimai database via a legacy DSN. The source database is read-only from Tockr's point of view.

## Mapping

- `kimai2_users` -> `users`, `user_roles`
- `kimai2_customers` -> `customers`
- `kimai2_projects` -> `projects`
- `kimai2_activities` -> `activities`
- `kimai2_timesheet` -> `timesheets`
- `kimai2_tags`, `kimai2_timesheet_tags` -> `tags`, `timesheet_tags`
- `kimai2_*_rates` -> `rates`
- `kimai2_invoices`, `kimai2_invoices_meta` -> `invoices`, `invoice_meta`
- team join tables -> team assignment tables

## Transformations

- PHP serialized roles are parsed into role names.
- Float money/rates are rounded to integer cents.
- Legacy unsupported fields are stored in `legacy_json` or metadata rows where useful.
- Password hashes are preserved when compatible; otherwise users require password reset.

## Rollback

Rollback is restore-from-backup. The migration never mutates the source Kimai database.

