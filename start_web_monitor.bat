@echo off
echo Starting ND-Go daemon with web interface...
echo ==========================================

echo Checking if daemon is already running...
tasklist /FI "IMAGENAME eq nd-go.exe" 2>NUL | find /I /N "nd-go.exe">NUL
if "%ERRORLEVEL%"=="0" (
    echo Daemon is running. Stopping it first...
    taskkill /F /IM nd-go.exe >NUL 2>&1
    timeout /t 2 /nobreak >NUL
)

echo Starting daemon...
start /B bin\nd-go.exe

echo Waiting for startup...
timeout /t 3 /nobreak >NUL

echo Checking ports...
netstat -ano | findstr ":8999" >NUL 2>&1
if "%ERRORLEVEL%"=="0" (
    echo [OK] TCP server listening on port 8999
) else (
    echo [ERROR] TCP port 8999 is not listening
)

netstat -ano | findstr ":8080" >NUL 2>&1
if "%ERRORLEVEL%"=="0" (
    echo [OK] Web interface available at: http://localhost:8080
) else (
    echo [ERROR] Web port 8080 is not listening
)

echo.
echo To stop the daemon, run: taskkill /F /IM nd-go.exe
echo.
pause
