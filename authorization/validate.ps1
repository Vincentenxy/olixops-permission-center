[CmdletBinding()]
param(
    [string]$ZedPath = 'zed'
)

$ErrorActionPreference = 'Stop'
$zed = Get-Command $ZedPath -CommandType Application -ErrorAction Stop
$fixtures = @(Get-ChildItem -LiteralPath (Join-Path $PSScriptRoot 'tests') -Filter '*.yaml' |
    Sort-Object Name | ForEach-Object FullName)
if ($fixtures.Count -eq 0) {
    throw 'No authorization fixtures found.'
}

Push-Location $PSScriptRoot
try {
    & $zed.Source validate --fail-on-warn (Join-Path $PSScriptRoot 'schema.zed') @fixtures
    if ($LASTEXITCODE -ne 0) {
        throw "SpiceDB model validation failed (exit $LASTEXITCODE)."
    }
}
finally {
    Pop-Location
}
