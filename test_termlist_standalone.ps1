# Standalone test for terminal list retrieval from 1C
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Testing Terminal List from 1C" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

$scriptPath = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $scriptPath

& ".\bin\test-termlist.exe"

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Read-Host "Press Enter to exit"

