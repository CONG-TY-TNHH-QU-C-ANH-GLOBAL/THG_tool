param(
  [string]$Version = $(if ($env:VERSION) { $env:VERSION } else { "dev" }),
  [string]$OutDir = $(if ($env:OUT_DIR) { $env:OUT_DIR } else { "data/downloads" })
)

$ErrorActionPreference = "Stop"

$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
$OutputDir = Join-Path $Root $OutDir
New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null

if (-not $env:GOCACHE) {
  $env:GOCACHE = Join-Path $Root ".gocache"
}

$env:CGO_ENABLED = "0"

$targets = @(
  @{ GOOS = "windows"; GOARCH = "amd64"; Name = "thg-login-windows.exe"; RuntimeName = "THG-Local-Runtime.exe"; Kit = "thg-local-kit-windows.zip" },
  @{ GOOS = "linux"; GOARCH = "amd64"; Name = "thg-login-linux"; RuntimeName = "thg-local-runtime"; Kit = "thg-local-kit-linux.zip" },
  @{ GOOS = "darwin"; GOARCH = "amd64"; Name = "thg-login-mac-intel"; RuntimeName = "thg-local-runtime"; Kit = "thg-local-kit-mac-intel.zip" },
  @{ GOOS = "darwin"; GOARCH = "arm64"; Name = "thg-login-mac-m1"; RuntimeName = "thg-local-runtime"; Kit = "thg-local-kit-mac-m1.zip" }
)

foreach ($target in $targets) {
  $env:GOOS = $target.GOOS
  $env:GOARCH = $target.GOARCH
  $out = Join-Path $OutputDir $target.Name
  Write-Host "Building $($target.Name) for $($target.GOOS)/$($target.GOARCH)"
  go build -buildvcs=false -ldflags="-s -w -X main.version=$Version" -o $out ./cmd/thg-login
}

$ExtensionDir = Join-Path $Root "local-connector-extension"
$ExtensionZip = Join-Path $OutputDir "thg-chrome-extension.zip"
if (Test-Path $ExtensionDir) {
  if (Test-Path $ExtensionZip) {
    Remove-Item -LiteralPath $ExtensionZip -Force
  }
  Write-Host "Packaging thg-chrome-extension.zip"
  Compress-Archive -Path (Join-Path $ExtensionDir "*") -DestinationPath $ExtensionZip -Force

  foreach ($target in $targets) {
    $kitZip = Join-Path $OutputDir $target.Kit
    $kitRoot = Join-Path $OutputDir ("kit-" + [IO.Path]::GetFileNameWithoutExtension($target.Kit))
    if (Test-Path $kitZip) {
      Remove-Item -LiteralPath $kitZip -Force
    }
    if (Test-Path $kitRoot) {
      Remove-Item -LiteralPath $kitRoot -Recurse -Force
    }
    New-Item -ItemType Directory -Path $kitRoot -Force | Out-Null
    Copy-Item -LiteralPath (Join-Path $OutputDir $target.Name) -Destination (Join-Path $kitRoot $target.RuntimeName) -Force
    if ($target.GOOS -eq "windows") {
      @"
@echo off
setlocal
cd /d "%~dp0"
title THG Local Runtime
echo ================================================
echo              THG LOCAL RUNTIME
echo ================================================
echo.
echo 1. Open the Browser dashboard and create a pairing code.
echo 2. Paste the pairing code here when prompted.
echo 3. Click "Mo Chrome local" in the dashboard.
echo 4. Log in to Facebook inside the Chrome window opened on this device.
echo 5. After Facebook opens, local Chrome moves away and the dashboard becomes the main workspace.
echo 6. Keep this window open while Facebook automation is running.
echo.
"%~dp0THG-Local-Runtime.exe" %*
set EXITCODE=%ERRORLEVEL%
echo.
echo THG Local Runtime stopped with exit code %EXITCODE%.
echo This window is kept open so you can read any message above.
pause
exit /b %EXITCODE%
"@ | Set-Content -LiteralPath (Join-Path $kitRoot "Start-THG-Local-Runtime.cmd") -Encoding ASCII
    } else {
      @"
#!/usr/bin/env bash
set -e
cd "`$(dirname "`$0")"
echo "================================================"
echo "             THG LOCAL RUNTIME"
echo "================================================"
echo
echo "1. Open the Browser dashboard and create a pairing code."
echo "2. Paste the pairing code here when prompted."
echo "3. Click \"Mo Chrome local\" in the dashboard."
echo "4. Log in to Facebook inside the Chrome window opened on this device."
echo "5. After Facebook opens, local Chrome moves away and the dashboard becomes the main workspace."
echo "6. Keep this terminal open while Facebook automation is running."
echo
chmod +x ./thg-local-runtime 2>/dev/null || true
./thg-local-runtime "`$@"
"@ | Set-Content -LiteralPath (Join-Path $kitRoot "start-thg-local-runtime.sh") -Encoding ASCII
    }
    @"
THG Local Kit

This package contains:
- THG Local Runtime: $($target.RuntimeName)
$(
if ($target.GOOS -eq "windows") {
"- Start script: Start-THG-Local-Runtime.cmd"
} else {
"- Start script: start-thg-local-runtime.sh"
}
)

Production flow:
1. Open the THG Browser dashboard.
2. Create a new pairing code.
3. Run the start script and paste the pairing code.
4. Keep the Runtime open, then click "Mo Chrome local" in the dashboard.
5. Log in to Facebook inside the Chrome window opened on this device.
6. After Facebook is ready, Local Runtime moves the Chrome window away.
7. The dashboard saves the account state and streams automation in Browser.

Security:
- Do not enter your Facebook password into THG.
- Runtime keeps the Facebook session in a local Chrome profile on this device.
- The dashboard stores account/session status, not your Facebook password.
"@ | Set-Content -LiteralPath (Join-Path $kitRoot "README.txt") -Encoding UTF8
    Write-Host "Packaging $($target.Kit)"
    Compress-Archive -Path (Join-Path $kitRoot "*") -DestinationPath $kitZip -Force
    Remove-Item -LiteralPath $kitRoot -Recurse -Force
  }

  $FrontendDownloads = Join-Path $Root "frontend\public\downloads"
  if (Test-Path (Join-Path $Root "frontend")) {
    New-Item -ItemType Directory -Path $FrontendDownloads -Force | Out-Null
    Copy-Item -LiteralPath $ExtensionZip -Destination (Join-Path $FrontendDownloads "thg-chrome-extension.zip") -Force
    Copy-Item -Path (Join-Path $OutputDir "thg-local-kit-*.zip") -Destination $FrontendDownloads -Force
  }
}

Write-Host "Local Connector builds written to $OutputDir"
