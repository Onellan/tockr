# Menu UX Improvements

## Implemented Direction

- Keep the sidebar visible and grouped because it is the fastest route map for the app.
- Move account/logout into an accessible dropdown to clean up the topbar.
- Move saved reports into an accessible dropdown to reduce report-page clutter.
- Use restrained styling: compact shadow, strong focus ring, clear hover state, and no heavy animation.
- Use a tiny dependency-free script for menu state.

## Accessibility

- Dropdown triggers are buttons with `aria-haspopup`, `aria-controls`, and `aria-expanded`.
- Menus are hidden by default and become visible only when open.
- Escape closes menus.
- Outside click closes menus.
- ArrowDown opens and focuses the first menu item.
- Links and form buttons remain keyboard reachable.

## Mobile

- Menus are positioned within the viewport and switch to full-width alignment where needed.
- Touch targets remain at least 36px high.
- Sidebar navigation remains a simple two-column layout on small screens.
