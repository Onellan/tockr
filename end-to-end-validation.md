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

- Anonymous pull passed after the GHCR package was made public:

```sh
docker logout ghcr.io
docker pull ghcr.io/onellan/tockr:latest
```

- Pulled digest: `sha256:5b43a03b19bd97507c1e6a185fd6554d599a69eb3cecf5af6baed62a2fc2c2ea`.

## Final Install Flow Confirmed

Confirmed with a fresh volume and the published GHCR image:

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

Validation results:

- `/healthz` returned healthy.
- `/app/data/.session_secret` was generated.
- `/app/data/.admin_password` was generated.
- Login with `admin@example.com` and the generated password returned `303 See Other`.
- Restart preserved the generated admin password.
- `/healthz` stayed healthy after restart.
