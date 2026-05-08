param(
  [string]$Version = $(if ($env:VERSION) { $env:VERSION } else { "dev" }),
  [string]$OutDir = $(if ($env:OUT_DIR) { $env:OUT_DIR } else { "dist/chrome-web-store" })
)

$ErrorActionPreference = "Stop"

$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
$OutputDir = Join-Path $Root $OutDir
# Source directory only. User installs should come from the official Web Store
# item or the CI-built beta zip, not from this repo folder directly.
$ExtensionDir = Join-Path $Root "local-connector-extension"
$ZipName = "thg-chrome-extension.zip"
$ExtensionZip = Join-Path $OutputDir $ZipName

New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null

if (-not (Test-Path $ExtensionDir)) {
  throw "Extension source directory not found: $ExtensionDir"
}

if (Test-Path $ExtensionZip) {
  Remove-Item -LiteralPath $ExtensionZip -Force
}

$StagingDir = Join-Path ([System.IO.Path]::GetTempPath()) ("thg-extension-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $StagingDir -Force | Out-Null

try {
  Copy-Item -Path (Join-Path $ExtensionDir "*") -Destination $StagingDir -Recurse -Force

  if ($Version -and $Version -ne "dev" -and $Version -ne "0") {
    if ($Version -match '^[0-9]+$') {
      $BuildSuffix = [int64]$Version
    } else {
      $Hex = $Version.TrimStart("v")
      if ($Hex.Length -gt 4) { $Hex = $Hex.Substring(0, 4) }
      if ($Hex -match '^[0-9a-fA-F]+$') {
        $BuildSuffix = [Convert]::ToInt64($Hex, 16)
      } else {
        Write-Warning "Cannot derive numeric build suffix from VERSION='$Version'; falling back to 0"
        $BuildSuffix = 0
      }
    }
    if ($BuildSuffix -gt 65535) {
      $BuildSuffix = $BuildSuffix % 65536
    }

    $ManifestPath = Join-Path $StagingDir "manifest.json"
    $Manifest = Get-Content -LiteralPath $ManifestPath -Raw | ConvertFrom-Json
    $Parts = @([string]$Manifest.version -split '\.')
    if ($Parts.Count -gt 3) { $Parts = $Parts[0..2] }
    $Manifest.version = (@($Parts) + [string]$BuildSuffix) -join '.'
    $Manifest | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath $ManifestPath -Encoding UTF8
    Write-Host "Stamped extension version: $($Manifest.version)"
  }

  Write-Host "Packaging THG Chrome Extension $Version"
  Compress-Archive -Path (Join-Path $StagingDir "*") -DestinationPath $ExtensionZip -Force
} finally {
  if (Test-Path $StagingDir) {
    Remove-Item -LiteralPath $StagingDir -Recurse -Force
  }
}

Write-Host "Chrome Extension package written to $ExtensionZip"
