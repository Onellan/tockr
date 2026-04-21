# Visual Improvement Plan

## Design Tokens

- Spacing: 4, 8, 12, 16, 20, 24, 32.
- Radius: 6px for controls, 8px for cards and panels.
- Shadows: one restrained elevation token for panels, cards, and tables.
- Typography: system UI stack, compact admin scale, no viewport-based type.
- Palette: neutral gray surfaces, dark blue-gray sidebar, teal primary action color, clear danger/success/warning semantic colors.

## Page Redesign Plan

- Login: production-grade sign-in card with concise product context.
- Main shell: Kimai-inspired persistent navigation, grouped by workflow area.
- Dashboard: metric cards plus clear active timer panel and operational focus.
- Timesheets: clear create-entry panel plus dense list table.
- Customers/projects/activities: consistent directory pages with create form and table.
- Reports: tabbed report grouping with concise rollup tables.
- Invoices: billing-focused creation toolbar and status/download table.
- Admin/users/settings: same page header, panel, and dense table patterns.

## Tradeoffs

- No icon library was added to preserve the no-build, low-dependency frontend.
- The design improves hierarchy and polish without adding animations or client state.
- Forms remain visible inline for speed and admin efficiency instead of modal-heavy workflows.

