# Phase 1 of STORE_SUBPACKAGE_REFACTOR: migrate every callsite of the
# 4 store-package helpers (parseSQLiteTime, retryOnBusy, utcDayKey,
# boolToInt) to the dbutil-exported names. Also adds the dbutil import
# to each affected file if missing.
#
# Limits scope to internal/store/ — these helpers are unexported in
# the original, so only same-package callers can reach them anyway.
# Idempotent: re-runnable safely.

$storeDir = Join-Path (Get-Location).Path 'internal\store'
$importPath = '"github.com/thg/scraper/internal/store/dbutil"'

$replacements = @(
    @{ from = '\bparseSQLiteTime\('; to = 'dbutil.ParseSQLiteTime(' }
    @{ from = '\bretryOnBusy\(';     to = 'dbutil.RetryOnBusy('     }
    @{ from = '\butcDayKey\(';       to = 'dbutil.UTCDayKey('       }
    @{ from = '\bboolToInt\(';       to = 'dbutil.BoolToInt('       }
)

$changed = 0
$skipped = 0

# Only operate on files DIRECTLY inside internal/store/, not subpackages.
Get-ChildItem -Path $storeDir -File -Filter '*.go' -Depth 0 | ForEach-Object {
    $p = $_.FullName
    $content = Get-Content -Raw -Path $p
    if (-not $content) {
        $skipped++
        return
    }
    $orig = $content

    # Apply each replacement. Skip files that are themselves inside
    # the dbutil subpackage (would self-reference via dbutil.* — wrong).
    foreach ($r in $replacements) {
        $content = [System.Text.RegularExpressions.Regex]::Replace($content, $r.from, $r.to)
    }

    # If any rewrite happened AND the file does not already import dbutil,
    # add the import. Heuristic: insert after the FIRST `import (` line.
    if ($content -ne $orig) {
        if ($content -notmatch [Regex]::Escape($importPath)) {
            # Find import block — match the first occurrence of `import (`
            # then inject our import on a new line right after.
            $content = [System.Text.RegularExpressions.Regex]::Replace(
                $content,
                '(?m)^import \(\s*$',
                "import (`r`n`t$importPath",
                1
            )
            # If the file uses single-import form (`import "foo"`), convert
            # to grouped form. Rare in this codebase; fall back to leaving
            # alone and warning.
            if ($content -notmatch [Regex]::Escape($importPath)) {
                Write-Warning "could not auto-add dbutil import: $p"
            }
        }
        Set-Content -Path $p -Value $content -NoNewline
        $changed++
        Write-Output "changed: $p"
    } else {
        $skipped++
    }
}

Write-Output ""
Write-Output "changed: $changed"
Write-Output "skipped: $skipped"
