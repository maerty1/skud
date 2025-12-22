@echo off
echo Testing ND-Go daemon on port 8999...

powershell -Command "& {try {$tcp = New-Object System.Net.Sockets.TcpClient('localhost', 8999); if ($tcp.Connected) {Write-Host 'SUCCESS: Daemon is listening on port 8999' -ForegroundColor Green; $tcp.Close()} else {Write-Host 'FAILED: Cannot connect' -ForegroundColor Red}} catch {Write-Host 'FAILED: Error connecting' -ForegroundColor Red}}"

echo.
echo Sending stats command...
powershell -Command "& {try {$tcp = New-Object System.Net.Sockets.TcpClient('localhost', 8999); $stream = $tcp.GetStream(); $writer = New-Object System.IO.StreamWriter($stream); $reader = New-Object System.IO.StreamReader($stream); $writer.WriteLine('stats'); $writer.Flush(); $response = $reader.ReadLine(); Write-Host 'Stats response:' $response -ForegroundColor Cyan; $tcp.Close()} catch {Write-Host 'Error sending command' -ForegroundColor Red}}"

pause
