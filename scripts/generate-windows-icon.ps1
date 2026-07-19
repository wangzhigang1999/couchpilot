param(
    [string]$OutputPath,
    [string]$PreviewPath
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = Join-Path $root "assets\windows\couchpilot.ico"
}
elseif (-not [System.IO.Path]::IsPathRooted($OutputPath)) {
    $OutputPath = Join-Path $root $OutputPath
}

if ([string]::IsNullOrWhiteSpace($PreviewPath)) {
    $PreviewPath = Join-Path $root ".cache\tray-icon-preview.png"
}
elseif (-not [System.IO.Path]::IsPathRooted($PreviewPath)) {
    $PreviewPath = Join-Path $root $PreviewPath
}

$env:GOPATH = Join-Path $root ".cache\gopath"
$env:GOMODCACHE = Join-Path $env:GOPATH "pkg\mod"
$env:GOCACHE = Join-Path $root ".cache\go-build"
$env:CGO_ENABLED = "0"

$generator = Join-Path $PSScriptRoot "icon-generator"
$source = Join-Path $root "assets\windows\couchpilot-tray.svg"

Push-Location $generator
try {
    & go run -mod=readonly . -source $source -output $OutputPath -preview $PreviewPath
    if ($LASTEXITCODE -ne 0) {
        throw "Windows icon generation failed with exit code $LASTEXITCODE"
    }
}
finally {
    Pop-Location
}

Write-Host "Generated: $OutputPath"
Write-Host "Preview:   $PreviewPath"
