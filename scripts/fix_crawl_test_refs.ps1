# One-shot: migrate type refs in internal/store/crawl_intents_test.go
# from legacy (store package) to new (crawl subpackage) names.

$f = 'internal\store\crawl_intents_test.go'
$c = Get-Content -Raw $f

$c = $c.Replace('CrawlIntent{', 'crawl.Intent{')
$c = $c.Replace('seedIntent(t *testing.T, db *Store, orgID int64) CrawlIntent', 'seedIntent(t *testing.T, db *Store, orgID int64) crawl.Intent')
$c = $c.Replace('CrawlIntentStatusActive', 'crawl.IntentStatusActive')
$c = $c.Replace('CrawlIntentStatusPaused', 'crawl.IntentStatusPaused')
$c = $c.Replace('CrawlIntentStatusArchived', 'crawl.IntentStatusArchived')
$c = $c.Replace('CrawlIntentStatusFailed', 'crawl.IntentStatusFailed')
$c = $c.Replace('CrawlIntentStatusCooldown', 'crawl.IntentStatusCooldown')
$c = $c -replace 'db\.getCrawlIntentByHash\(', 'db.Crawl().GetIntentByHash('

# Add crawl import — find the import block and add it.
if ($c -notmatch 'internal/store/crawl') {
    $c = $c -replace '(?m)^(\s*"testing"\s*$)', "`$1`r`n`r`n`t`"github.com/thg/scraper/internal/store/crawl`""
}

Set-Content -Path $f -Value $c -NoNewline
Write-Output "migrated: $f"
