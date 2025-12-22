# СКД - Система контроля доступа - Скрипт запуска
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  СКД - Система контроля доступа" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Check if binary exists
if (-not (Test-Path "bin\skd.exe")) {
    Write-Host "[ERROR] Binary not found: bin\skd.exe" -ForegroundColor Red
    Write-Host ""
    Write-Host "Please build the project first:" -ForegroundColor Yellow
    Write-Host "  go build -o bin\skd.exe cmd\main.go" -ForegroundColor Yellow
    Write-Host ""
    Read-Host "Press Enter to exit"
    exit 1
}

Write-Host "Запуск СКД (Система контроля доступа)..." -ForegroundColor Green
Write-Host ""
Write-Host "TCP Server: 0.0.0.0:8999" -ForegroundColor Yellow
Write-Host "Web Interface: http://localhost:8080" -ForegroundColor Yellow
Write-Host ""
Write-Host "Press Ctrl+C to stop" -ForegroundColor Gray
Write-Host ""

# Run the daemon
& "bin\skd.exe"


