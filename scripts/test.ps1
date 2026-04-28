param(
	[switch]$Security
)

$ErrorActionPreference = "Stop"

function Invoke-CISecurityChecks {
	Write-Host "[security] Running gosec"
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	$gosecPath = Join-Path (go env GOPATH) "bin\gosec.exe"
	& $gosecPath ./...

	Write-Host "[security] Running go vet"
	go vet ./...

	Write-Host "[security] Checking go.mod/go.sum tidy state"
	go mod tidy
	git diff --exit-code go.mod go.sum

	Write-Host "[security] Running govulncheck"
	go install golang.org/x/vuln/cmd/govulncheck@latest
	$govulncheckPath = Join-Path (go env GOPATH) "bin\govulncheck.exe"
	& $govulncheckPath ./...

	Write-Host "[security] Running gitleaks"
	$gitleaksCmd = Get-Command gitleaks -ErrorAction SilentlyContinue
	if ($null -ne $gitleaksCmd) {
		gitleaks detect --source . --no-banner --redact --exit-code 1
	}
	else {
		docker run --rm -v "${PWD}:/repo" ghcr.io/gitleaks/gitleaks:latest detect --source=/repo --no-banner --redact --exit-code=1
	}
}

docker compose version | Out-Null
docker compose run --build --rm test

if ($Security) {
	Invoke-CISecurityChecks
}
