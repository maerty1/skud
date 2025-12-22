# PowerShell script to install Go
Write-Host "Installing Go 1.25.5 for Windows..."

# Create temp directory
$tempDir = "$env:TEMP\go_install"
New-Item -ItemType Directory -Force -Path $tempDir | Out-Null

# Download Go installer
$url = "https://go.dev/dl/go1.25.5.windows-amd64.msi"
$installerPath = "$tempDir\go-installer.msi"

Write-Host "Downloading Go installer..."
try {
    Invoke-WebRequest -Uri $url -OutFile $installerPath -UseBasicParsing
    Write-Host "Download completed."
} catch {
    Write-Host "Download failed: $_"
    exit 1
}

# Install Go
Write-Host "Installing Go..."
try {
    Start-Process msiexec.exe -ArgumentList "/i `"$installerPath`" /quiet /norestart" -Wait
    Write-Host "Installation completed."
} catch {
    Write-Host "Installation failed: $_"
    exit 1
}

# Refresh environment variables
$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")

# Verify installation
Write-Host "Verifying installation..."
try {
    $goVersion = & go version 2>$null
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Go installed successfully: $goVersion"
    } else {
        Write-Host "Go installation verification failed."
        exit 1
    }
} catch {
    Write-Host "Go command not found. Please restart PowerShell or add Go to PATH."
    Write-Host "Go bin directory is typically: C:\Program Files\Go\bin"
    exit 1
}

# Clean up
Remove-Item -Path $tempDir -Recurse -Force

Write-Host "Go installation completed successfully!"
