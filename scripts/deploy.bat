@echo off
REM ============================================
REM THG Agentic Scraper — Deploy from Laptop
REM ============================================
REM Usage: deploy.bat <SERVER_IP> <SSH_KEY_PATH>
REM Example: deploy.bat 129.154.xx.xx C:\Users\ACER\.ssh\oracle_key

set SERVER_IP=%1
set SSH_KEY=%2

if "%SERVER_IP%"=="" (
    echo Usage: deploy.bat ^<SERVER_IP^> ^<SSH_KEY_PATH^>
    echo Example: deploy.bat 129.154.xx.xx C:\Users\ACER\.ssh\oracle_key
    exit /b 1
)

if "%SSH_KEY%"=="" (
    echo Usage: deploy.bat ^<SERVER_IP^> ^<SSH_KEY_PATH^>
    exit /b 1
)

echo.
echo 🕷️  THG Agentic Scraper — Deploy
echo ================================
echo Target: %SERVER_IP%
echo.

REM 1. Cross-compile for Linux ARM64
echo 📦 Building binary for Linux ARM64...
set GOOS=linux
set GOARCH=arm64
set CGO_ENABLED=0
go build -ldflags="-s -w" -o dist\scraper .\cmd\scraper
if %ERRORLEVEL% NEQ 0 (
    echo ❌ Build failed!
    exit /b 1
)
echo ✅ Binary built: dist\scraper

REM 2. Upload binary
echo 📤 Uploading binary to server...
scp -i %SSH_KEY% dist\scraper ubuntu@%SERVER_IP%:/opt/thg-scraper/scraper
if %ERRORLEVEL% NEQ 0 (
    echo ❌ Upload failed!
    exit /b 1
)

REM 3. Upload .env if exists
if exist .env (
    echo 📤 Uploading .env...
    scp -i %SSH_KEY% .env ubuntu@%SERVER_IP%:/opt/thg-scraper/.env
)

REM 4. Restart service
echo 🔄 Restarting service...
ssh -i %SSH_KEY% ubuntu@%SERVER_IP% "chmod +x /opt/thg-scraper/scraper && sudo systemctl restart thg-scraper && sleep 2 && sudo systemctl status thg-scraper --no-pager -l"

echo.
echo ✅ Deploy complete!
echo 🖥️  Web UI: http://%SERVER_IP%:8080
echo 🤖 Telegram: Send /start to your bot
echo.
