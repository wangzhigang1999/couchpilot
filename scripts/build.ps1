$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$env:GOPATH = Join-Path $root ".cache\gopath"
$env:GOMODCACHE = Join-Path $env:GOPATH "pkg\mod"
$env:GOCACHE = Join-Path $root ".cache\go-build"

Push-Location $root
try {
    go mod download
    go test ./...
    go vet ./...
    New-Item -ItemType Directory -Force bin | Out-Null
    go build -trimpath -ldflags "-s -w" -o bin\couchpilot.exe .\cmd\couchpilot
    Write-Host "Built: $root\bin\couchpilot.exe"
}
finally {
    Pop-Location
}
