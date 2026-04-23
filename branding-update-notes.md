# Branding Update Notes

## Audit Summary

- The previous visual system centered on a dark slate sidebar and teal primary color, which made the app feel heavier than intended for the current product direction.
- The shared application head in `web/templates/templates.go` is the canonical source for favicon links, stylesheet versioning, manifest wiring, and browser `theme-color`.
- The manifest in `web/static/site.webmanifest` still used the previous background and brand color.
- Most component styling already flows through root CSS variables in `web/static/style.css`, making a full palette refresh practical without changing layout or workflows.
- A handful of components used hard-coded older tints and shadows, especially around alerts, timer state, role cards, row hover states, and the login background.

## Implemented Brand Refresh

- Applied the Bright SaaS Blue palette across the root design tokens.
- Lightened the sidebar and topbar so navigation feels cleaner and less visually dense.
- Updated primary, hover, focus, warning, success, and danger treatments to the new palette.
- Softened shadows and slightly modernized radius values for a cleaner SaaS look.
- Retuned timer state, alerts, info callouts, tables, inputs, buttons, and auth background to match the brighter visual system.

## Favicon And Metadata

- Replaced the favicon/image set with a new blue-brand Tockr icon family.
- Updated shared favicon, Apple touch icon, and manifest references with a fresh cache-busting version.
- Updated browser `theme-color` to `#2F80ED`.
- Updated manifest `background_color` and `theme_color` to match the refreshed brand.

## Validation Checklist

- [ ] Regenerate and verify favicon assets.
- [ ] Run `go test ./...`.
- [ ] Run `go vet ./...`.
- [ ] Validate key screens locally after restart.
- [ ] Push to GitHub and confirm full CI success.
