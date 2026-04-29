.PHONY: build build-linux build-agent build-worker run run-worker clean test ci validate

# Build backend API server for the current OS. cmd/scraper owns the Fiber API.
build:
	go build -ldflags="-s -w" -o scraper.exe ./cmd/scraper

# Cross-compile production Go binaries for Linux VPS.
build-linux:
	if not exist dist mkdir dist
	set GOOS=linux&& set GOARCH=amd64&& go build -ldflags="-s -w" -o dist/scraper ./cmd/scraper
	set GOOS=linux&& set GOARCH=amd64&& go build -ldflags="-s -w" -o dist/thg-worker ./cmd/worker

# Build THG Login Agent for all platforms (staff download these to their machines).
build-agent:
	if not exist data\downloads mkdir data\downloads
	set GOOS=windows&& set GOARCH=amd64&& go build -ldflags="-s -w" -o data/downloads/thg-login-windows.exe ./cmd/thg-login
	set GOOS=darwin&& set GOARCH=amd64&& go build -ldflags="-s -w" -o data/downloads/thg-login-mac-intel ./cmd/thg-login
	set GOOS=darwin&& set GOARCH=arm64&& go build -ldflags="-s -w" -o data/downloads/thg-login-mac-m1 ./cmd/thg-login
	set GOOS=linux&& set GOARCH=amd64&& go build -ldflags="-s -w" -o data/downloads/thg-login-linux ./cmd/thg-login

# Build worker for the current OS.
build-worker:
	if not exist bin mkdir bin
	go build -o bin/worker.exe ./cmd/worker/

# Run API server locally.
run:
	go run ./cmd/scraper

# Run worker locally.
run-worker:
	DB_PATH=data/scraper.db go run ./cmd/worker/

# Run tests.
test:
	go test ./internal/... -v

# Validate architecture and compile current production entrypoints.
validate:
	go vet ./...
	go build ./cmd/scraper/ ./cmd/worker/

# Full local CI pipeline.
ci: validate test build build-worker
	@echo "CI: ALL CHECKS PASSED"

# Clean build artifacts.
clean:
	del /Q scraper.exe 2>nul
	rmdir /S /Q dist 2>nul
	rmdir /S /Q bin 2>nul
