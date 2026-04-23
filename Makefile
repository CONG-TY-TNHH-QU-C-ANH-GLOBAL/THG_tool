.PHONY: build build-linux run clean test

# Build for current OS
build:
	go build -ldflags="-s -w" -o scraper.exe ./cmd/scraper

# Cross-compile for Linux VPS
build-linux:
	set GOOS=linux&& set GOARCH=amd64&& go build -ldflags="-s -w" -o dist/scraper ./cmd/scraper

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
