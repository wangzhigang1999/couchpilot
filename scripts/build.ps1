param(
    [string]$Version = "0.2.0",
    [string]$OutputPath
)

$ErrorActionPreference = "Stop"

if ($Version -notmatch '^\d+\.\d+\.\d+(\.\d+)?$') {
    throw "Version must contain three or four numeric components; received '$Version'"
}

$root = Split-Path -Parent $PSScriptRoot
$env:GOPATH = Join-Path $root ".cache\gopath"
$env:GOMODCACHE = Join-Path $env:GOPATH "pkg\mod"
$env:GOCACHE = Join-Path $root ".cache\go-build"
$env:CGO_ENABLED = "0"

$icon = Join-Path $root "assets\windows\couchpilot.ico"
$resourcePrefix = Join-Path $root "cmd\couchpilot\zz_couchpilot_resources"
$resourceObject = "${resourcePrefix}_windows_amd64.syso"
if ([string]::IsNullOrWhiteSpace($OutputPath)) {
    $output = Join-Path $root "bin\couchpilot.exe"
}
elseif ([System.IO.Path]::IsPathRooted($OutputPath)) {
    $output = [System.IO.Path]::GetFullPath($OutputPath)
}
else {
    $output = [System.IO.Path]::GetFullPath((Join-Path $root $OutputPath))
}

function Invoke-Checked {
    param(
        [Parameter(Mandatory = $true)]
        [scriptblock]$Command,
        [Parameter(Mandatory = $true)]
        [string]$Description
    )

    & $Command
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) {
        throw "$Description failed with exit code $exitCode"
    }
}

Push-Location $root
try {
    if (-not (Test-Path -LiteralPath $icon -PathType Leaf)) {
        throw "Missing Windows icon: $icon"
    }

    Invoke-Checked { go mod download } "Go dependency download"
    Invoke-Checked { go test ./... } "Go tests"
    Invoke-Checked { go vet ./... } "Go static checks"

    if (Test-Path -LiteralPath $resourceObject) {
        Remove-Item -LiteralPath $resourceObject -Force
    }
    try {
        Invoke-Checked {
            go run github.com/tc-hib/go-winres@v0.3.3 simply `
                --arch amd64 `
                --out $resourcePrefix `
                --manifest cli `
                --product-version $Version `
                --file-version $Version `
                --product-name "CouchPilot" `
                --file-description "CouchPilot gamepad desktop controller" `
                --original-filename "couchpilot.exe" `
                --copyright "Copyright 2026 CouchPilot contributors" `
                --icon $icon
        } "Windows resource generation"

        if (-not (Test-Path -LiteralPath $resourceObject -PathType Leaf)) {
            throw "Windows resource generator did not create $resourceObject"
        }

        New-Item -ItemType Directory -Force (Split-Path -Parent $output) | Out-Null
        Invoke-Checked {
            go build -trimpath -ldflags "-s -w" -o $output .\cmd\couchpilot
        } "CouchPilot build"
    }
    finally {
        if (Test-Path -LiteralPath $resourceObject) {
            Remove-Item -LiteralPath $resourceObject -Force
        }
    }

    Write-Host "Built: $output"
}
finally {
    Pop-Location
}
