# Favicon Fix Plan

## Assets

- `web/static/favicon.ico`
- `web/static/favicon-16x16.png`
- `web/static/favicon-32x32.png`
- `web/static/apple-touch-icon.png`
- `web/static/icon-192.png`
- `web/static/icon-512.png`
- `web/static/site.webmanifest`

## HTML Head

The shared layout will include:

- root `favicon.ico` link with a version query
- PNG icon links with sizes
- Apple touch icon link
- manifest link
- `theme-color`

## Routing

- `/favicon.ico` serves the ICO asset instead of `204`.
- `/apple-touch-icon.png`, `/favicon-16x16.png`, `/favicon-32x32.png`, and `/site.webmanifest` are supported as root aliases.
- `/static/*` continues to serve the canonical files.

## Cache Strategy

- Versioned query strings in the head invalidate normal browser icon caches after asset changes.
- Root aliases remain for browsers that request conventional paths directly.
