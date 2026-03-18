[CmdletBinding()]
param(
    [switch]$Dev
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Assert-Command {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name
    )

    if (-not (Get-Command -Name $Name -ErrorAction SilentlyContinue)) {
        throw "required command '$Name' was not found in PATH"
    }
}

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$rootDir = Resolve-Path (Join-Path $scriptDir "..")
$outputExe = Join-Path $rootDir "build\bin\FFmpegFree.exe"
$buildScript = Join-Path $scriptDir "build.ps1"

Set-Location $rootDir

if ($Dev) {
    Assert-Command -Name "wails"
    Write-Host "Starting development mode..." -ForegroundColor Cyan
    wails dev
    exit $LASTEXITCODE
}

if (-not (Test-Path $outputExe)) {
    if (-not (Test-Path $buildScript)) {
        throw "build script not found: $buildScript"
    }

    Write-Host "Binary not found, building first..." -ForegroundColor Yellow
    & $buildScript
}

if (-not (Test-Path $outputExe)) {
    throw "cannot start app because executable was not found: $outputExe"
}

Write-Host "Starting app..." -ForegroundColor Green
Start-Process -FilePath $outputExe -WorkingDirectory (Split-Path -Parent $outputExe)
