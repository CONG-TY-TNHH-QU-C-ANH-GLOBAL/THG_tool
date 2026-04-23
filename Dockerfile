# Multi-stage build for ARM64 (Oracle Cloud Always Free)
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

ARG TARGETARCH
ARG TARGETOS

RUN apk add --no-cache git

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o scraper ./cmd/scraper

# ── Runtime image ─────────────────────────────────────────────────────────
FROM --platform=$TARGETPLATFORM ubuntu:22.04

# Install Chromium and dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    chromium-browser \
    fonts-liberation \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Create dedicated non-root user
RUN groupadd --gid 10001 scraper \
 && useradd  --uid 10001 --gid scraper --no-create-home --shell /usr/sbin/nologin scraper

WORKDIR /app

# Copy binary owned by root, executable by all
COPY --from=builder --chown=root:root /app/scraper ./scraper
RUN chmod 755 /app/scraper

# Data directory writable by service user only
RUN mkdir -p /app/data/logs /app/data/profiles /app/data/backups \
 && chown -R scraper:scraper /app/data \
 && chmod 750 /app/data

# Declare volume so Docker knows this is persistent state
VOLUME ["/app/data"]

USER scraper

ENV CHROME_PATH=/usr/bin/chromium-browser
ENV WEB_PORT=8080
ENV DB_PATH=/app/data/scraper.db
ENV PROFILE_DIR=/app/data/profiles
ENV MAX_WORKERS=1

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/api/stats || exit 1

ENTRYPOINT ["./scraper"]
