# CI/CD Pipeline

## Current-state audit

- Stack: Go 1.26 server-rendered web app with SQLite via `modernc.org/sqlite`.
- Validation command: `go test ./...`.
- Build command: `go build ./cmd/app`.
- Dockerfile: multi-stage Go build into Alpine runtime, static binary, non-root runtime user, `/healthz` healthcheck.
- Compose: local development publishes host `8029` to container `8080`.
- Existing app workflows: none under `.github/workflows`.
- Existing blocker: ARM64 Docker builds worked but were slow because Go compiled inside an emulated ARM64 builder.
- Existing inefficiency: `.dockerignore` allowed local output and tool cache directories into the Docker build context.

## Failure analysis

No existing GitHub Actions workflow was failing because the repository did not have an app CI workflow yet. Local verification found these release risks:

- Docker context was larger than needed.
- ARM64 build was slow under emulation.
- Published image instructions were missing.
- No automated container startup smoke test existed.

## Implemented design

The workflow in `.github/workflows/ci.yml` has three jobs:

1. `validate`: checkout, set up Go from `go.mod`, run `go test ./...`, and build the app binary.
2. `container-smoke`: build a local `linux/amd64` Docker image, start it, and verify `/healthz`.
3. `docker-image`: build `linux/amd64,linux/arm64` with Buildx and publish to GHCR on non-PR events.

Publication only runs after validation and smoke testing pass.

## Registry and tagging plan

Registry: GitHub Container Registry.

Image name:

```text
ghcr.io/<owner>/<repo>
```

Tags:

- `latest` for the default branch.
- `sha-<short-sha>` for traceable builds.
- `vX.Y.Z`, `X.Y`, and `X` for semantic release tags such as `v1.2.3`.

GHCR uses `GITHUB_TOKEN` in the workflow with `packages: write`, so no personal access token is required. Public packages can be pulled anonymously after package visibility is set to public in GHCR.

## Raspberry Pi image strategy

- Build `linux/arm64` for Raspberry Pi OS 64-bit.
- Build `linux/amd64` for local/server installs.
- Do not build `linux/arm/v7` by default. The project documentation recommends 64-bit Raspberry Pi OS, and adding 32-bit ARM would increase CI time and compatibility risk without a current requirement.

## Efficiency plan

- Use concurrency cancellation so outdated branch runs stop.
- Split validation, smoke, and publish gates.
- Use Buildx `type=gha` cache with a stable scope.
- Cross-compile Go using `$BUILDPLATFORM`, `$TARGETOS`, and `$TARGETARCH` to avoid slow QEMU compilation.
- Avoid QEMU setup because the Dockerfile no longer needs target-platform `RUN` instructions.
- Ignore markdown/deployment-only changes for branch and PR CI.
- Avoid a large matrix. Multi-platform support is handled by one Buildx build.

## Security plan

- Use job-level permissions.
- Publish with `GITHUB_TOKEN`, not a PAT.
- Run the container as non-root UID/GID `65532`.
- Keep secrets out of Docker build args and image layers.
- Require validation and smoke tests before publish.

## References

- GitHub Packages permissions: https://docs.github.com/en/packages/learn-github-packages/about-permissions-for-github-packages
- Publishing packages with GitHub Actions: https://docs.github.com/packages/managing-github-packages-using-github-actions-workflows/publishing-and-installing-a-package-with-github-actions
- Docker multi-platform GitHub Actions: https://docs.docker.com/build/ci/github-actions/multi-platform/
- Docker GitHub Actions cache backend: https://docs.docker.com/build/cache/backends/gha/
- GitHub Actions concurrency: https://docs.github.com/en/actions/using-jobs/using-concurrency
