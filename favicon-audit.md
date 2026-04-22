# Favicon Audit

## Current State

- `/favicon.ico` returned `204 No Content`.
- The shared HTML layout did not include favicon links.
- No PNG, Apple touch icon, manifest, or theme-color metadata existed.
- There were no conflicting favicon declarations, but there was also no canonical strategy.

## Risks

- Chrome and other browsers may cache the empty favicon response.
- Browser tabs may show a generic icon.
- Installed/mobile contexts have no manifest or touch icon.
- Production deployments inherit the missing favicon because the shared layout owns the head.

## Fix Direction

- Serve real root-level favicon aliases for browser defaults.
- Add cache-busted icon references in the shared layout.
- Add PNG icons for modern browsers and Apple touch icon support.
- Add a small web manifest with icon references.
- Keep one canonical brand mark: the Tockr `T` on the primary green background.
