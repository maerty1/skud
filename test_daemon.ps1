# Test script for ND-Go daemon
Write-Host "Testing ND-Go daemon on port 8999..."

try {
    $tcpClient = New-Object System.Net.Sockets.TcpClient('localhost', 8999)
    if ($tcpClient.Connected) {
        Write-Host "✅ SUCCESS: Daemon is listening on port 8999" -ForegroundColor Green

        $stream = $tcpClient.GetStream()
        $writer = New-Object System.IO.StreamWriter($stream)
        $reader = New-Object System.IO.StreamReader($stream)

        # Test stats command
        $writer.WriteLine('stats')
        $writer.Flush()
        Start-Sleep -Milliseconds 500
        $response = $reader.ReadLine()
        Write-Host "📊 Stats response: $response" -ForegroundColor Cyan

        # Test termlist command
        $writer.WriteLine('termlist')
        $writer.Flush()
        Start-Sleep -Milliseconds 500
        $termlistResponse = $reader.ReadLine()
        Write-Host "📋 Termlist response: $termlistResponse" -ForegroundColor Cyan

        $tcpClient.Close()
        Write-Host "✅ All tests completed successfully!" -ForegroundColor Green
    }
} catch {
    Write-Host "❌ FAILED: Cannot connect to daemon on port 8999" -ForegroundColor Red
    Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Red
}
