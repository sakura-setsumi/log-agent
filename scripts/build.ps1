[CmdletBinding()]
param(
    [switch]$SkipTests
)

$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$distDirectory = Join-Path $projectRoot 'dist'
$output = Join-Path $distDirectory 'dozzle-ops.exe'
$go = Join-Path ${env:ProgramFiles} 'Go\bin\go.exe'
$env:GOCACHE = Join-Path ([System.IO.Path]::GetTempPath()) 'log-agent-go-cache'

if (-not (Test-Path -LiteralPath $go)) {
    throw "Go executable was not found: $go"
}

New-Item -ItemType Directory -Force -Path $distDirectory | Out-Null
Push-Location $projectRoot
try {
    if (-not $SkipTests) {
        & $go test ./...
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    }

    & $go build -buildvcs=false -o $output .
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
    Pop-Location
}

Write-Host "Built $output"
