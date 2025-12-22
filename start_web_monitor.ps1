# PowerShell script to start ND-Go daemon with web monitoring
Write-Host "Starting ND-Go daemon with web interface..."
Write-Host "=========================================="

# Check if daemon is already running
$existingProcess = Get-Process -Name "nd-go" -ErrorAction SilentlyContinue
if ($existingProcess) {
    Write-Host "Daemon is already running. Stopping it first..."
    Stop-Process -Name "nd-go" -Force
    Start-Sleep -Seconds 2
}

# Start the daemon
Write-Host "Starting daemon..."
Start-Process -FilePath "$PSScriptRoot\bin\nd-go.exe" -NoNewWindow

# Wait a bit for startup
Start-Sleep -Seconds 3

# Check if ports are listening
$tcpPort = Test-NetConnection -ComputerName localhost -Port 8999 -WarningAction SilentlyContinue
$webPort = Test-NetConnection -ComputerName localhost -Port 8080 -WarningAction SilentlyContinue

if ($tcpPort.TcpTestSucceeded -and $webPort.TcpTestSucceeded) {
    Write-Host "✅ Daemon started successfully!" -ForegroundColor Green
    Write-Host "📡 TCP server listening on port 8999" -ForegroundColor Cyan
    Write-Host "🌐 Web interface available at: http://localhost:8080" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "To stop the daemon, run: Stop-Process -Name 'nd-go' -Force" -ForegroundColor Yellow
} else {
    Write-Host "❌ Failed to start daemon or ports are not listening" -ForegroundColor Red
    if (-not $tcpPort.TcpTestSucceeded) {
        Write-Host "   TCP port 8999 is not listening" -ForegroundColor Red
    }
    if (-not $webPort.TcpTestSucceeded) {
        Write-Host "   Web port 8080 is not listening" -ForegroundColor Red
    }
}
