# Menu UX Improvements

## Design Direction

The menu should remain Kimai-inspired: persistent left navigation, grouped business workflows, admin tools separated from daily work, and server-rendered pages. The improvement is polish, not reinvention.

## Improvements

- Use permission-aware groups so users never see destinations that fail immediately.
- Add active indicators that are visible but restrained.
- Improve group spacing and link touch targets.
- Add consistent hover and focus states for links, buttons, and tabs.
- Separate account identity from the logout button.
- Add accessible labels to sidebar, primary navigation, topbar, and report tabs.
- Keep mobile navigation lightweight with wrapping groups instead of JavaScript drawers.

## Tradeoffs

- No icon dependency is added. The app stays dependency-light and Raspberry Pi friendly.
- No dropdown account menu is added. A direct logout action is faster, clearer, and avoids JavaScript for now.
- No collapsible sidebar is added. The current information architecture is small enough that a persistent rail is simpler and more reliable.
