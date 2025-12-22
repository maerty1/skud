@echo off
echo Testing terminal list retrieval from 1C...
echo.

echo Starting daemon...
start /B bin\nd-go.exe

timeout /t 3 /nobreak >nul

echo.
echo Sending 'system check_db' command...
echo system check_db | ncat localhost 8999

echo.
echo Stopping daemon...
taskkill /F /IM nd-go.exe >nul 2>&1

echo.
echo Test completed!

