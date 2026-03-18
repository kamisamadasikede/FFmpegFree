[CmdletBinding()]
param(
    [switch]$SkipInstall
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
$frontendDir = Join-Path $rootDir "frontend"
$resourceScript = Join-Path $rootDir "copy-resources.ps1"
$outputExe = Join-Path $rootDir "build\bin\FFmpegFree.exe"

Set-Location $rootDir

Assert-Command -Name "go"
Assert-Command -Name "npm"
Assert-Command -Name "wails"

if (-not (Test-Path $frontendDir)) {
    throw "frontend directory not found: $frontendDir"
}

$nodeModulesDir = Join-Path $frontendDir "node_modules"
if (-not $SkipInstall -and -not (Test-Path $nodeModulesDir)) {
    Write-Host "Installing frontend dependencies..." -ForegroundColor Cyan
    Push-Location $frontendDir
    try {
        npm install
        if ($LASTEXITCODE -ne 0) {
            throw "npm install failed with exit code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }
}

Write-Host "Building desktop app..." -ForegroundColor Cyan
wails build
if ($LASTEXITCODE -ne 0) {
    throw "wails build failed with exit code $LASTEXITCODE"
}

if (Test-Path $resourceScript) {
    Write-Host "Copying ffmpeg resources..." -ForegroundColor Cyan
    & $resourceScript
}

if (Test-Path $outputExe) {
    Write-Host "Build finished: $outputExe" -ForegroundColor Green
}
else {
    Write-Warning "build completed but output executable was not found at $outputExe"
}
