# Rewrite Plan

## Goal

Replace Kimai's Symfony/PHP stack with a lean Go application that preserves the core time-tracking workflows for up to 30 concurrent users on Raspberry Pi 4B hardware.

## Keep

- Users, roles, permissions.
- Customers, projects, activities.
- Timesheets, active timers, manual entries.
- Tags and rates.
- Reports for dashboard, customers, activities, projects, and users.
- Basic invoicing, invoice metadata, and invoice download.
- Useful JSON APIs with pagination.
- Webhooks as the lightweight replacement for plugins.

## Simplify

- Role model becomes four built-in roles with persisted permission overrides.
- Invoice rendering stores simple HTML/CSV-friendly documents instead of full Office/PDF template engines.
- Metadata is key/value text rather than dynamic Symfony field definitions.
- Reports prioritize operational summaries over highly customizable widgets.

## Defer Or Drop

- SAML, LDAP, 2FA, plugin loading, Webpack, Symfony bundles, full Kimai invoice renderer parity, calendar views, advanced widgets, bookmarks, and working-time contracts.

## Phases

1. Foundation: Go module, config, router, SQLite, migrations, seed admin.
2. Security: sessions, CSRF, password hashing, authorization.
3. Core data: users, customers, projects, activities, tags, rates.
4. Time tracking: list, filter, pagination, start/stop, manual entries, future-time policy.
5. Reporting and invoices.
6. Webhooks and compact JSON API.
7. Migration utilities and deployment artifacts.

