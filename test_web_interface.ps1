# Test script for ND-Go web interface
Write-Host "Testing ND-Go web interface..." -ForegroundColor Cyan
Write-Host "================================"

# Test main page
Write-Host "Testing main web page..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "http://localhost:8080/" -TimeoutSec 5
    if ($response.StatusCode -eq 200 -and $response.Content -match "ND-Go Daemon Status") {
        Write-Host "✅ Main page loads successfully" -ForegroundColor Green
    } else {
        Write-Host "❌ Main page returned unexpected content" -ForegroundColor Red
    }
} catch {
    Write-Host "❌ Main page failed to load: $($_.Exception.Message)" -ForegroundColor Red
}

# Test stats API
Write-Host "Testing stats API..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "http://localhost:8080/api/stats" -TimeoutSec 5
    $stats = $response.Content | ConvertFrom-Json
    if ($stats.running -eq $true -and $stats.uptime -gt 0) {
        Write-Host "✅ Stats API working" -ForegroundColor Green
        Write-Host "   Uptime: $([math]::Round($stats.uptime, 1)) seconds" -ForegroundColor Gray
        Write-Host "   Connections: $($stats.connections)" -ForegroundColor Gray
        Write-Host "   Sessions: $($stats.sessions)" -ForegroundColor Gray
    } else {
        Write-Host "❌ Stats API returned invalid data" -ForegroundColor Red
    }
} catch {
    Write-Host "❌ Stats API failed: $($_.Exception.Message)" -ForegroundColor Red
}

# Test connections API
Write-Host "Testing connections API..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "http://localhost:8080/api/connections" -TimeoutSec 5
    $connections = $response.Content | ConvertFrom-Json
    if ($connections -is [array]) {
        Write-Host "✅ Connections API working" -ForegroundColor Green
        Write-Host "   Active connections: $($connections.Count)" -ForegroundColor Gray
    } else {
        Write-Host "❌ Connections API returned invalid data" -ForegroundColor Red
    }
} catch {
    Write-Host "❌ Connections API failed: $($_.Exception.Message)" -ForegroundColor Red
}

# Test sessions API
Write-Host "Testing sessions API..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "http://localhost:8080/api/sessions" -TimeoutSec 5
    $sessions = $response.Content | ConvertFrom-Json
    if ($sessions -is [array]) {
        Write-Host "✅ Sessions API working" -ForegroundColor Green
        Write-Host "   Active sessions: $($sessions.Count)" -ForegroundColor Gray
    } else {
        Write-Host "❌ Sessions API returned invalid data" -ForegroundColor Red
    }
} catch {
    Write-Host "❌ Sessions API failed: $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host ""
Write-Host "Web interface test completed!" -ForegroundColor Cyan
Write-Host "Access the web interface at: http://localhost:8080" -ForegroundColor Cyan
