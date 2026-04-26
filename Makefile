.PHONY: build build-linux build-agent run clean test build-api build-worker ci validate run-api run-worker

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

# Build new API + Worker binaries
build-api:
	go build -o bin/api.exe ./cmd/api/

build-worker:
	go build -o bin/worker.exe ./cmd/worker/

# Validate architecture (no forbidden imports, compile clean)
validate:
	go vet ./...
	go build ./cmd/api/ ./cmd/worker/

# Full CI pipeline: validate → test → build
ci: validate test build-api build-worker
	@echo "CI: ALL CHECKS PASSED"

# Run new API server
run-api:
	DB_PATH=data/scraper.db API_PORT=8080 go run ./cmd/api/

# Run new worker
run-worker:
	DB_PATH=data/scraper.db go run ./cmd/worker/

# Clean build artifacts
clean:
	del /Q scraper.exe 2>nul
	rmdir /S /Q dist 2>nul
	rmdir /S /Q bin 2>nul
