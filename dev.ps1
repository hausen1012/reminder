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
    # 开发默认开 debug：本地只看得到 SQL、访问日志与投递中间步骤才方便排查。
    # 想临时降噪就先 `$env:LOG_LEVEL = 'info'` 再运行本脚本（外部值优先于这里的默认值）。
    "`$Host.UI.RawUI.WindowTitle = '$BackendTitle'; if (-not `$env:LOG_LEVEL) { `$env:LOG_LEVEL = 'debug' }; `$env:LOG_FILE = '$RootDir\logs'; `$env:DB_PATH = '$RootDir\data\reminder.db'; cd '$RootDir\backend'; go run ./cmd/server/"
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