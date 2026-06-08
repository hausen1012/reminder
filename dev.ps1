$RootDir = $PSScriptRoot

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Host "Error: Go not found, install from https://go.dev" -ForegroundColor Red
    exit 1
}
if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
    Write-Host "Error: Node.js not found, install from https://nodejs.org" -ForegroundColor Red
    exit 1
}

if (-not (Test-Path "$RootDir\frontend\node_modules")) {
    Write-Host "Installing frontend dependencies..." -ForegroundColor Yellow
    Push-Location "$RootDir\frontend"
    npm install
    Pop-Location
}

$BackendTitle = "Reminder Backend (Go)"
$FrontendTitle = "Reminder Frontend (Vite)"

Write-Host "===================================" -ForegroundColor Cyan
Write-Host "  Reminder Dev Server Starting..." -ForegroundColor Cyan
Write-Host "===================================" -ForegroundColor Cyan
Write-Host "Press Enter in this window to stop all servers" -ForegroundColor Cyan
Write-Host ""

$BackendArgs = @(
    "-NoLogo"
    "-NoExit"
    "-Command"
    "`$Host.UI.RawUI.WindowTitle = '$BackendTitle'; `$env:LOG_FILE = '$RootDir\logs'; cd '$RootDir\backend'; go run ./cmd/server/"
)
$backendJob = Start-Process -WindowStyle Normal -PassThru -FilePath "powershell" -ArgumentList $BackendArgs

Start-Sleep -Seconds 2

$FrontendArgs = @(
    "-NoLogo"
    "-NoExit"
    "-Command"
    "`$Host.UI.RawUI.WindowTitle = '$FrontendTitle'; cd '$RootDir\frontend'; npx vite --host"
)
$frontendJob = Start-Process -WindowStyle Normal -PassThru -FilePath "powershell" -ArgumentList $FrontendArgs

Write-Host "Servers started, press Enter to stop all..." -ForegroundColor Green
$null = Read-Host

Write-Host "Stopping servers..." -ForegroundColor Yellow
if (-not $backendJob.HasExited) { $backendJob.Kill() }
if (-not $frontendJob.HasExited) { $frontendJob.Kill() }
Write-Host "Stopped." -ForegroundColor Green