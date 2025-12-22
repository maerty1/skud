@echo off
echo Installing Go for Windows...
echo.

echo Step 1: Downloading Go installer...
powershell -Command "& {Invoke-WebRequest -Uri 'https://go.dev/dl/go1.25.5.windows-amd64.msi' -OutFile '%TEMP%\go-installer.msi'}"

if errorlevel 1 (
    echo Download failed. Please download manually from: https://go.dev/dl/
    pause
    exit /b 1
)

echo Download completed.
echo.

echo Step 2: Installing Go...
msiexec /i "%TEMP%\go-installer.msi" /quiet /norestart

if errorlevel 1 (
    echo Installation failed.
    pause
    exit /b 1
)

echo Installation completed.
echo.

echo Step 3: Verifying installation...
go version

if errorlevel 1 (
    echo.
    echo Go installation completed, but 'go' command not found in PATH.
    echo Please restart Command Prompt or add to PATH manually:
    echo C:\Program Files\Go\bin
    echo.
    pause
    exit /b 1
)

echo.
echo Go installed successfully!
echo You can now build the project with: go build cmd/main.go
echo.

pause
