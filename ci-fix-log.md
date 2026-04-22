# CI Fix Log

## Planned CI Behavior

- Pull requests validate Go tests, app build, and a local Docker smoke image.
- Pushes to `main` validate, smoke test, then publish `linux/amd64` and
  `linux/arm64` images to GHCR.
- Release tags publish semantic version tags.

## Changes Made

- Updated the container smoke test to omit `TOCKR_SESSION_SECRET` and
  `TOCKR_ADMIN_PASSWORD`.
- Added smoke assertions that generated secret files exist.
- Added a smoke login check using the generated admin password.

## Validation Log

- Local validation results are recorded in `end-to-end-validation.md`.
- GitHub Actions results must be added after pushing and iterating CI.
