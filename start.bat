@echo off
echo ========================================
echo   СКД - Система контроля доступа
echo ========================================
echo.

REM Check if binary exists
if not exist bin\skd.exe (
    echo [ERROR] Binary not found: bin\skd.exe
    echo.
    echo Please build the project first:
    echo   go build -o bin\skd.exe cmd/main.go
    echo.
    pause
    exit /b 1
)

echo Запуск СКД (Система контроля доступа)...
echo.
echo TCP Server: 0.0.0.0:8999
echo Web Interface: http://localhost:8080
echo.
echo Press Ctrl+C to stop
echo.

REM Run the daemon
bin\skd.exe

pause


