[CmdletBinding(SupportsShouldProcess)]
param(
    [switch]$IncludeProjectCache
)

$ErrorActionPreference = 'Stop'
$projectRoot = [System.IO.Path]::GetFullPath((Split-Path -Parent $PSScriptRoot)).TrimEnd('\', '/')
$targets = @([System.IO.Path]::Combine($projectRoot, 'dist'))
if ($IncludeProjectCache) {
    $targets += [System.IO.Path]::Combine($projectRoot, '.gocache')
}

foreach ($target in $targets) {
    $resolved = [System.IO.Path]::GetFullPath($target).TrimEnd('\', '/')
    if (-not $resolved.StartsWith("$projectRoot\", [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Refusing to clean a path outside the project: $resolved"
    }
    if ((Test-Path -LiteralPath $resolved) -and $PSCmdlet.ShouldProcess($resolved, 'Remove generated build artifacts')) {
        Remove-Item -LiteralPath $resolved -Recurse -Force
    }
}
