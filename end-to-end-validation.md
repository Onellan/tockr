# End-To-End Validation

This file records the exact validation performed for the automated Docker
install flow.

## Local Validation

- `go test ./...`: passed after restoring global workspace switcher rendering.
- `go vet ./...`: passed.
- `docker build -t tockr:automated-install-check .`: passed.
- Local Docker run from a fresh `tockr-auto-e2e-data` volume: passed.
- `/healthz` returned `{"status":"ok"}` from the test container.
- `/app/data/.session_secret` was generated and non-empty.
- `/app/data/.admin_password` was generated and at least 24 characters.
- Login with `admin@example.com` and the generated password returned the
  expected `303 See Other`.
- Generated admin password persisted across container restart.

## GitHub Actions Validation

Pending.

## Published Image Validation

Pending.

## Final Install Flow Confirmed

Pending.
