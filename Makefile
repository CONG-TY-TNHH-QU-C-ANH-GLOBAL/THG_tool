.PHONY: build build-linux build-agent run clean test

# Build server for current OS
build:
	go build -ldflags="-s -w" -o scraper.exe ./cmd/scraper

# Cross-compile server for Linux VPS
build-linux:
	set GOOS=linux&& set GOARCH=amd64&& go build -ldflags="-s -w" -o dist/scraper ./cmd/scraper

# Build THG Login Agent for all platforms (staff download these to their machines)
build-agent:
	if not exist data\downloads mkdir data\downloads
	set GOOS=windows&& set GOARCH=amd64&& go build -ldflags="-s -w" -o data/downloads/thg-login-windows.exe ./cmd/thg-login
	set GOOS=darwin&& set GOARCH=amd64&& go build -ldflags="-s -w" -o data/downloads/thg-login-mac-intel ./cmd/thg-login
	set GOOS=darwin&& set GOARCH=arm64&& go build -ldflags="-s -w" -o data/downloads/thg-login-mac-m1 ./cmd/thg-login
	set GOOS=linux&& set GOARCH=amd64&& go build -ldflags="-s -w" -o data/downloads/thg-login-linux ./cmd/thg-login

# Run locally
run:
	go run ./cmd/scraper

# Run tests
test:
	go test ./internal/... -v

# Clean build artifacts
clean:
	del /Q scraper.exe 2>nul
	rmdir /S /Q dist 2>nul
