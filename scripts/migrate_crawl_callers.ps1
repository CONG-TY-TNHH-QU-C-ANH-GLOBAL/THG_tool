# Phase 3 of STORE_SUBPACKAGE_REFACTOR — clean-cut migration of all
# crawl-domain callers. Updates `.MethodName(` → `.Crawl().NewMethodName(`
# across internal/ + cmd/. Replaces type references
# `store.CrawlIntent...` → `crawl.Intent...` and `store.PrivateFile`
# → `crawl.PrivateFile`. Adds `internal/store/crawl` import where
# the file already imports `internal/store`.

$crawlImport = '"github.com/thg/scraper/internal/store/crawl"'
$storeImportLine = '"github.com/thg/scraper/internal/store"'

# Method renames: methodName → Crawl().newMethodName
# Format: from-name (just the method name, regex word-boundary applied) → replacement
$methodRenames = @(
    # crawl_intents methods — drop CrawlIntent prefix
    @{ from = 'UpsertCrawlIntent';              to = 'Crawl().UpsertIntent' }
    @{ from = 'ClaimDueCrawlIntents';           to = 'Crawl().ClaimDueIntents' }
    @{ from = 'MarkCrawlIntentRunResult';       to = 'Crawl().MarkIntentRunResult' }
    @{ from = 'SetCrawlIntentStatus';           to = 'Crawl().SetIntentStatus' }
    @{ from = 'UpdateCrawlIntentCursor';        to = 'Crawl().UpdateIntentCursor' }
    @{ from = 'AdvanceCrawlIntentCursor';       to = 'Crawl().AdvanceIntentCursor' }
    @{ from = 'CountActiveCrawlIntentsForOrg';  to = 'Crawl().CountActiveIntentsForOrg' }
    @{ from = 'ListCrawlIntentsForOrg';         to = 'Crawl().ListIntentsForOrg' }
    @{ from = 'SetCrawlIntentEnabled';          to = 'Crawl().SetIntentEnabled' }

    # groups methods — keep name, add Crawl()
    @{ from = 'AddGroup';            to = 'Crawl().AddGroup' }
    @{ from = 'GroupExistsByURL';    to = 'Crawl().GroupExistsByURL' }
    @{ from = 'GetActiveGroups';     to = 'Crawl().GetActiveGroups' }
    @{ from = 'GetAllGroups';        to = 'Crawl().GetAllGroups' }
    @{ from = 'UpdateGroupLastScan'; to = 'Crawl().UpdateGroupLastScan' }
    @{ from = 'ToggleGroup';         to = 'Crawl().ToggleGroup' }
    @{ from = 'DeleteGroup';         to = 'Crawl().DeleteGroup' }

    # group_quality methods
    @{ from = 'UpsertGroupQuality';        to = 'Crawl().UpsertGroupQuality' }
    @{ from = 'GetGroupQuality';           to = 'Crawl().GetGroupQuality' }
    @{ from = 'GetQualityGroupsForDomain'; to = 'Crawl().GetQualityGroupsForDomain' }
    @{ from = 'GetAllScoredGroups';        to = 'Crawl().GetAllScoredGroups' }
    @{ from = 'MarkGroupWhitelist';        to = 'Crawl().MarkGroupWhitelist' }
    @{ from = 'MarkGroupBlacklist';        to = 'Crawl().MarkGroupBlacklist' }
    @{ from = 'UpdateGroupYield';          to = 'Crawl().UpdateGroupYield' }
    @{ from = 'UpdateGroupLastPost';       to = 'Crawl().UpdateGroupLastPost' }
    @{ from = 'GetUnscoredGroups';         to = 'Crawl().GetUnscoredGroups' }

    # posts methods
    @{ from = 'InsertPost';       to = 'Crawl().InsertPost' }
    @{ from = 'GetRecentPosts';   to = 'Crawl().GetRecentPosts' }
    @{ from = 'DeletePost';       to = 'Crawl().DeletePost' }
    @{ from = 'DeleteAllPosts';   to = 'Crawl().DeleteAllPosts' }
    @{ from = 'InsertComment';    to = 'Crawl().InsertComment' }
    @{ from = 'InsertPostsBatch'; to = 'Crawl().InsertPostsBatch' }

    # private_files methods
    @{ from = 'InsertPrivateFile'; to = 'Crawl().InsertPrivateFile' }
    @{ from = 'GetPrivateFiles';   to = 'Crawl().GetPrivateFiles' }
    @{ from = 'DeletePrivateFile'; to = 'Crawl().DeletePrivateFile' }
)

# Type renames: store.XxX → crawl.YyY (exact-string match, not regex)
$typeRenames = @(
    @{ from = 'store.CrawlIntentStatusActive';   to = 'crawl.IntentStatusActive' }
    @{ from = 'store.CrawlIntentStatusPaused';   to = 'crawl.IntentStatusPaused' }
    @{ from = 'store.CrawlIntentStatusArchived'; to = 'crawl.IntentStatusArchived' }
    @{ from = 'store.CrawlIntentStatusFailed';   to = 'crawl.IntentStatusFailed' }
    @{ from = 'store.CrawlIntentStatusCooldown'; to = 'crawl.IntentStatusCooldown' }
    @{ from = 'store.IsValidCrawlIntentStatus';  to = 'crawl.IsValidIntentStatus' }
    @{ from = 'store.CrawlIntent';               to = 'crawl.Intent' }
    @{ from = 'store.PrivateFile';               to = 'crawl.PrivateFile' }
)

$repoRoot = (Get-Location).Path
$changed = 0
$skipped = 0

# Walk all *.go files in internal/ and cmd/ — skip the crawl
# subpackage itself (it would self-rewrite).
Get-ChildItem -Path 'internal','cmd' -Recurse -Filter '*.go' | Where-Object {
    $_.FullName -notlike "*\internal\store\crawl\*"
} | ForEach-Object {
    $p = $_.FullName
    $content = Get-Content -Raw -Path $p
    if (-not $content) {
        $skipped++
        return
    }
    $orig = $content

    foreach ($m in $methodRenames) {
        # Match `.<name>(` with word-boundary on the preceding `.` so
        # we don't accidentally match unrelated tokens.
        $pattern = '\.' + $m.from + '\('
        $replacement = '.' + $m.to + '('
        $content = [System.Text.RegularExpressions.Regex]::Replace($content, $pattern, $replacement)
    }
    foreach ($t in $typeRenames) {
        $content = $content.Replace($t.from, $t.to)
    }

    # If we rewrote anything and the file imports `internal/store`,
    # also add the `internal/store/crawl` import next to it.
    if ($content -ne $orig) {
        if ($content -match [Regex]::Escape($storeImportLine) -and
            $content -notmatch [Regex]::Escape($crawlImport)) {
            # Insert crawl import on the line BEFORE the store import to
            # keep alphabetical-ish ordering (`crawl` < `store` sub-path).
            $content = $content -replace [Regex]::Escape($storeImportLine), ($crawlImport + "`r`n`t" + $storeImportLine)
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
