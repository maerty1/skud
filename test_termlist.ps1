# Test script for getting terminal list from 1C
# Usage: .\test_termlist.ps1

Write-Host "Testing terminal list retrieval from 1C..." -ForegroundColor Cyan

# Start daemon in background
Write-Host "Starting daemon..." -ForegroundColor Yellow
$daemon = Start-Process -FilePath ".\bin\nd-go.exe" -PassThru -WindowStyle Hidden

# Wait for daemon to start
Start-Sleep -Seconds 3

# Test command via TCP
Write-Host "`nSending 'system check_db' command..." -ForegroundColor Yellow

try {
    $tcpClient = New-Object System.Net.Sockets.TcpClient("localhost", 8999)
    $stream = $tcpClient.GetStream()
    
    $command = "system check_db`n"
    $bytes = [System.Text.Encoding]::ASCII.GetBytes($command)
    $stream.Write($bytes, 0, $bytes.Length)
    
    # Read response
    $buffer = New-Object byte[] 4096
    $bytesRead = $stream.Read($buffer, 0, $buffer.Length)
    $response = [System.Text.Encoding]::ASCII.GetString($buffer, 0, $bytesRead)
    
    Write-Host "`nResponse from daemon:" -ForegroundColor Green
    Write-Host $response
    
    $stream.Close()
    $tcpClient.Close()
} catch {
    Write-Host "Error: $_" -ForegroundColor Red
} finally {
    # Stop daemon
    Write-Host "`nStopping daemon..." -ForegroundColor Yellow
    if ($daemon -and !$daemon.HasExited) {
        Stop-Process -Id $daemon.Id -Force
    }
}

Write-Host "`nTest completed!" -ForegroundColor Cyan

