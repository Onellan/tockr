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

- Commit pushed: `04e7434`.
- Workflow run: `24803564445`.
- Result: success.
- Jobs passed:
  - Validate Go application.
  - Build and smoke test container.
  - Build and publish Docker image.

## Published Image Validation

- `docker pull ghcr.io/onellan/tockr:latest` currently returns
  `unauthorized`.
- The CI publish job succeeded, so the image exists, but GHCR package
  visibility is private. Anonymous end-user pull requires the package to be
  changed to public in GitHub package settings.

## Final Install Flow Confirmed

Blocked until GHCR package visibility is public, then rerun:

```sh
docker pull ghcr.io/onellan/tockr:latest
docker volume create tockr-published-e2e-data
docker run -d --name tockr-published-e2e \
  -p 18082:8080 \
  -v tockr-published-e2e-data:/app/data \
  ghcr.io/onellan/tockr:latest
curl -fsS http://localhost:18082/healthz
docker exec tockr-published-e2e cat /app/data/.admin_password
```
