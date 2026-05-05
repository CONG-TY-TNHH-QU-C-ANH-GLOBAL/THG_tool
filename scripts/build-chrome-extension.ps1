param(
  [string]$Version = $(if ($env:VERSION) { $env:VERSION } else { "dev" }),
  [string]$OutDir = $(if ($env:OUT_DIR) { $env:OUT_DIR } else { "dist/chrome-web-store" })
)

$ErrorActionPreference = "Stop"

$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
$OutputDir = Join-Path $Root $OutDir
$ExtensionDir = Join-Path $Root "local-connector-extension"
$ZipName = "thg-chrome-extension.zip"
$ExtensionZip = Join-Path $OutputDir $ZipName

New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null

if (-not (Test-Path $ExtensionDir)) {
  throw "Extension directory not found: $ExtensionDir"
}

if (Test-Path $ExtensionZip) {
  Remove-Item -LiteralPath $ExtensionZip -Force
}

Write-Host "Packaging THG Chrome Extension $Version"
Compress-Archive -Path (Join-Path $ExtensionDir "*") -DestinationPath $ExtensionZip -Force

Write-Host "Chrome Extension package written to $ExtensionZip"
