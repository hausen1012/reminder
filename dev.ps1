<#
.SYNOPSIS
    Reminder dev launcher.

.DESCRIPTION
    .\dev.ps1          dev mode: Go backend + Vite dev server (hot reload)
    .\dev.ps1 build    production-ish: build frontend, embed, single binary, run
    .\dev.ps1 test     run all tests

    Env overrides: PORT (backend), WEB_PORT (vite), WEB_HOST (vite), DB_PATH, LOG_LEVEL.

.EXAMPLE
    .\dev.ps1

.EXAMPLE
    $env:PORT='8766'; .\dev.ps1
#>
[CmdletBinding()]
param(
    [ValidateSet('dev', 'build', 'test')]
    [string]$Mode = 'dev'
)

$ErrorActionPreference = 'Stop'

$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $Root

if (-not $env:PORT)           { $env:PORT = '8766' }
if (-not $env:WEB_PORT)       { $env:WEB_PORT = '8765' }
if (-not $env:WEB_HOST)       { $env:WEB_HOST = '0.0.0.0' }
if (-not $env:DB_PATH)        { $env:DB_PATH = Join-Path $Root 'data\reminder.db' }
if (-not $env:LOG_LEVEL)      { $env:LOG_LEVEL = 'debug' }

# PowerShell 5.1 writes native stderr as errors; keep going anyway.
$PSNativeCommandUseErrorActionPreference = $false

# Reset terminal input mode on exit so Ctrl+C never leaves console in raw mode
if (-not ([System.Management.Automation.PSTypeName]'Reminder.ConsoleHelper').Type) {
    try {
        Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
namespace Reminder {
    public static class ConsoleHelper {
        [DllImport("kernel32.dll", SetLastError = true)]
        private static extern IntPtr GetStdHandle(int nStdHandle);
        [DllImport("kernel32.dll", SetLastError = true)]
        private static extern bool SetConsoleMode(IntPtr hConsoleHandle, uint dwMode);

        public static void ResetInputMode() {
            try {
                IntPtr hIn = GetStdHandle(-10); // STD_INPUT_HANDLE
                if (hIn != IntPtr.Zero) {
                    // ENABLE_PROCESSED_INPUT (0x1) | ENABLE_LINE_INPUT (0x2) | ENABLE_ECHO_INPUT (0x4) | 
                    // ENABLE_MOUSE_INPUT (0x10) | ENABLE_INSERT_MODE (0x20) | ENABLE_QUICK_EDIT_MODE (0x40) | ENABLE_EXTENDED_FLAGS (0x80)
                    SetConsoleMode(hIn, 0x01F7);
                }
            } catch {}
        }
    }
}
'@ -ErrorAction SilentlyContinue
    } catch {}
}

function Reset-ConsoleState {
    try {
        if (([System.Management.Automation.PSTypeName]'Reminder.ConsoleHelper').Type) {
            [Reminder.ConsoleHelper]::ResetInputMode()
        }
    } catch {}

    try {
        [Console]::TreatControlCAsInput = $false
        [Console]::ResetColor()
        $flushLimit = 50
        while ([Console]::KeyAvailable -and $flushLimit -gt 0) {
            [void][Console]::ReadKey($true)
            $flushLimit--
        }
    } catch {}
}

function Stop-PortProcess([int]$Port) {
    try {
        $lines = netstat -ano -p tcp | Select-String ":$Port\s"
        $killed = @{}
        foreach ($line in $lines) {
            $parts = ($line.ToString().Trim() -split '\s+')
            if ($parts.Count -ge 5 -and $parts[1] -match ":$Port$") {
                $targetPid = 0
                if ([int]::TryParse($parts[-1], [ref]$targetPid)) {
                    if ($targetPid -gt 0 -and $targetPid -ne $PID -and -not $killed.ContainsKey($targetPid)) {
                        $killed[$targetPid] = $true
                        try {
                            & taskkill /PID $targetPid /T /F *>$null
                        } catch {}
                    }
                }
            }
        }
    } catch {}
}

function Require-Tool([string]$Name) {
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "'$Name' not found in PATH"
    }
}

function Write-Banner([string]$Text) {
    Write-Host ''
    Write-Host '--------------------------------------------' -ForegroundColor DarkGray
    Write-Host "  $Text" -ForegroundColor Cyan
    Write-Host '--------------------------------------------' -ForegroundColor DarkGray
    Write-Host ''
}

function Ensure-NodeModules {
    $nm = Join-Path $Root 'frontend\node_modules'
    $vite = Join-Path $nm 'vite\bin\vite.js'
    if ((Test-Path $nm) -and (Test-Path $vite)) {
        return
    }
    Write-Banner 'Install frontend dependencies'
    Push-Location (Join-Path $Root 'frontend')
    try {
        if (Test-Path 'package-lock.json') {
            npm ci --no-audit --no-fund
        } else {
            npm install --no-audit --no-fund
        }
    } finally { Pop-Location }
    if ($LASTEXITCODE -ne 0) { throw 'npm install failed' }
}

function Ensure-DataDir {
    $dataDir = Split-Path -Parent $env:DB_PATH
    if ($dataDir -and -not (Test-Path $dataDir)) {
        New-Item -ItemType Directory -Force $dataDir | Out-Null
    }
}

function Invoke-Dev {
    Require-Tool 'go'
    Require-Tool 'node'
    Ensure-NodeModules
    Ensure-DataDir

    Stop-PortProcess -Port ([int]$env:PORT)
    Stop-PortProcess -Port ([int]$env:WEB_PORT)

    Write-Banner "Dev mode  frontend :$($env:WEB_PORT)  backend :$($env:PORT)"
    Write-Host "Open http://localhost:$($env:WEB_PORT)"
    Write-Host ''

    $backend = Start-Process -FilePath 'go' `
        -ArgumentList @('run', './cmd/server') `
        -WorkingDirectory (Join-Path $Root 'backend') `
        -NoNewWindow -PassThru

    # Launch vite directly with node to bypass cmd.exe and batch prompt on Ctrl+C
    $viteCli = Join-Path $Root 'frontend\node_modules\vite\bin\vite.js'
    if (Test-Path $viteCli) {
        $frontend = Start-Process -FilePath 'node' `
            -ArgumentList @($viteCli) `
            -WorkingDirectory (Join-Path $Root 'frontend') `
            -NoNewWindow -PassThru
    } else {
        $frontend = Start-Process -FilePath 'cmd.exe' `
            -ArgumentList @('/c', 'npm', 'run', 'dev') `
            -WorkingDirectory (Join-Path $Root 'frontend') `
            -NoNewWindow -PassThru
    }

    Write-Host "backend pid=$($backend.Id)  frontend pid=$($frontend.Id)" -ForegroundColor DarkGray

    try {
        while (-not $backend.HasExited -and -not $frontend.HasExited) {
            Start-Sleep -Milliseconds 300
        }
    }
    finally {
        Write-Host ''
        Write-Host 'Stopping services...' -ForegroundColor DarkGray
        foreach ($p in @($backend, $frontend)) {
            if ($p) {
                try {
                    if (-not $p.HasExited) {
                        & taskkill /PID $p.Id /T /F *>$null
                    }
                } catch {}
            }
        }
        Stop-PortProcess -Port ([int]$env:PORT)
        Stop-PortProcess -Port ([int]$env:WEB_PORT)
        Reset-ConsoleState
        Write-Host 'Stopped cleanly.' -ForegroundColor Green
    }
}

function Invoke-Build {
    Require-Tool 'go'
    Require-Tool 'npm'
    Ensure-NodeModules

    Write-Banner 'Build frontend'
    Push-Location (Join-Path $Root 'frontend')
    try { npm run build } finally { Pop-Location }
    if ($LASTEXITCODE -ne 0) { throw 'frontend build failed' }

    Write-Banner 'Embed web assets'
    $webDir = Join-Path $Root 'backend\cmd\server\web'
    if (-not (Test-Path $webDir)) {
        New-Item -ItemType Directory -Force $webDir | Out-Null
    }
    Get-ChildItem -Path $webDir -Exclude '.gitkeep' | Remove-Item -Recurse -Force
    Copy-Item -Path (Join-Path $Root 'frontend\dist\*') -Destination $webDir -Recurse -Force

    Write-Banner 'Compile binary'
    $buildDir = Join-Path $Root 'build'
    if (-not (Test-Path $buildDir)) {
        New-Item -ItemType Directory -Force $buildDir | Out-Null
    }
    Push-Location (Join-Path $Root 'backend')
    try {
        $version = try { (git describe --tags --abbrev=0 2>$null) } catch { $null }
        if (-not $version) { $version = 'dev' }
        $buildTime = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
        $ldflags = "-s -w -X github.com/reminder/backend/internal/version.Version=$version -X github.com/reminder/backend/internal/version.BuildTime=$buildTime"
        go build -trimpath -ldflags $ldflags -o ..\build\server.exe ./cmd/server
    } finally { Pop-Location }
    if ($LASTEXITCODE -ne 0) { throw 'go build failed' }

    Write-Banner "Start  http://localhost:8765"
    Ensure-DataDir
    & (Join-Path $buildDir 'server.exe')
}

function Invoke-Test {
    Require-Tool 'go'
    Require-Tool 'npm'

    Write-Banner 'Go tests'
    Push-Location (Join-Path $Root 'backend')
    try { go test ./... } finally { Pop-Location }
    if ($LASTEXITCODE -ne 0) { throw 'go tests failed' }

    Write-Banner 'Frontend tests'
    Push-Location (Join-Path $Root 'frontend')
    try {
        npm test
    } finally { Pop-Location }
}

switch ($Mode) {
    'dev'   { Invoke-Dev }
    'build' { Invoke-Build }
    'test'  { Invoke-Test }
}
