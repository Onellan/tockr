# Menu Architecture Audit

## Current Structure

- Primary navigation is rendered from `primaryNav` in `web/templates/templates.go`.
- Admin navigation is rendered from `adminNav` and uses the same permission-aware renderer.
- Active state is path-based through `isActivePath`.
- The topbar has workspace context and a visible account/logout area.
- Reports use tabs plus filter and save-report forms.
- Table row actions are currently sparse: project dashboard and invoice download.

## Findings

- Sidebar grouping is clear and should remain visible; it is the primary information architecture.
- The account/logout controls are visually exposed in the topbar and are the strongest dropdown candidate.
- Saved reports currently render as a table, which is bulky for a small secondary list.
- Report filters are still primary actions and should stay visible.
- Row actions are not dense enough yet to justify per-row overflow menus.
- Permission filtering already happens server-side before menu links render.

## Recommendation

- Keep sidebar groups as normal visible navigation.
- Add a compact account dropdown in the topbar.
- Add a saved-report dropdown on the reports page.
- Avoid dropdowns for main nav, timer start/stop, invoice download, and project dashboard links until those areas gain multiple secondary actions.
