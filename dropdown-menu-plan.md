# Dropdown Menu Plan

## Dropdowns To Add

| Location | Dropdown | Rationale |
|---|---|---|
| Topbar | Account menu | Moves logout and quick account links under one predictable control while keeping identity visible. |
| Reports | Saved reports menu | Saved report choices are secondary and should not consume table space. |

## Dropdowns Not Added

- Sidebar groups stay visible for discoverability and power-user speed.
- Timer actions stay visible because they are the main workflow.
- Invoice/project row actions stay direct because each row only has one meaningful action today.
- Filter fields stay visible because hiding filters would slow reporting workflows.

## Behavior

- Button toggles menu with `aria-expanded`.
- Enter/Space work through native button behavior.
- Escape closes open menus and restores focus.
- Outside click closes menus.
- ArrowDown opens a closed menu and focuses the first menu item.
- Menus use server-rendered HTML and a tiny progressive-enhancement script.
