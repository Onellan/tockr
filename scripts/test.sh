#!/usr/bin/env sh
set -eu

SECURITY=0
if [ "${1:-}" = "--security" ]; then
	SECURITY=1
fi

run_ci_security_checks() {
	echo "[security] Running gosec"
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	"$(go env GOPATH)/bin/gosec" ./...

	echo "[security] Running go vet"
	go vet ./...

	echo "[security] Checking go.mod/go.sum tidy state"
	go mod tidy
	git diff --exit-code go.mod go.sum

	echo "[security] Running govulncheck"
	go install golang.org/x/vuln/cmd/govulncheck@latest
	"$(go env GOPATH)/bin/govulncheck" ./...

	echo "[security] Running gitleaks"
	if command -v gitleaks >/dev/null 2>&1; then
		gitleaks detect --source . --no-banner --redact --exit-code 1
	else
		docker run --rm -v "$(pwd):/repo" ghcr.io/gitleaks/gitleaks:latest detect --source=/repo --no-banner --redact --exit-code=1
	fi
}

docker compose version >/dev/null
docker compose run --build --rm test

if [ "$SECURITY" -eq 1 ]; then
	run_ci_security_checks
fi
