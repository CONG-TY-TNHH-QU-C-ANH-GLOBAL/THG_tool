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
  @{ GOOS = "windows"; GOARCH = "amd64"; Name = "thg-login-windows.exe" },
  @{ GOOS = "linux"; GOARCH = "amd64"; Name = "thg-login-linux" },
  @{ GOOS = "darwin"; GOARCH = "amd64"; Name = "thg-login-mac-intel" },
  @{ GOOS = "darwin"; GOARCH = "arm64"; Name = "thg-login-mac-m1" }
)

foreach ($target in $targets) {
  $env:GOOS = $target.GOOS
  $env:GOARCH = $target.GOARCH
  $out = Join-Path $OutputDir $target.Name
  Write-Host "Building $($target.Name) for $($target.GOOS)/$($target.GOARCH)"
  go build -buildvcs=false -ldflags="-s -w -X main.version=$Version" -o $out ./cmd/thg-login
}

Write-Host "Local Connector builds written to $OutputDir"
