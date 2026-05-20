# Tags every internal/store/*.go file with a `// Domain: <name>` header comment.
# Phase 0 of the STORE_SUBPACKAGE_REFACTOR. Idempotent: skips files that
# already carry a Domain tag.

# Mapping: filename → domain (sourced from DOMAINS.md audit, 2026-05-21).
$mapping = @{
    'accounts.go'                       = 'identities'
    'accounts_identity_test.go'         = 'identities'
    'accounts_rbac_test.go'             = 'identities'
    'action_ledger.go'                  = 'coordination'
    'action_ledger_test.go'             = 'coordination'
    'action_policy.go'                  = 'outbound'
    'agent_tokens.go'                   = 'identities'
    'app_store.go'                      = 'app'
    'backup.go'                         = 'infra'
    'behaviour_profile.go'              = 'coordination'
    'behaviour_profile_test.go'         = 'coordination'
    'career_jobs.go'                    = 'app'
    'classification_log.go'             = 'leads'
    'clear_db_test.go'                  = 'dbutil'
    'connector_commands.go'             = 'connectors'
    'connector_ownership.go'            = 'connectors'
    'connector_pairing.go'              = 'connectors'
    'connector_streams.go'              = 'connectors'
    'context_niches.go'                 = 'leads'
    'crawl_intents.go'                  = 'crawl'
    'crawl_intents_test.go'             = 'crawl'
    'data_sources.go'                   = 'crawl'
    'dedup.go'                          = 'dbutil'
    'dialect.go'                        = 'dbutil'
    'dialect_postgres.go'               = 'dbutil'
    'dialect_sqlite.go'                 = 'dbutil'
    'dialect_test.go'                   = 'dbutil'
    'engagement_reconcile.go'           = 'coordination'
    'engagement_reconcile_test.go'      = 'coordination'
    'execution_attempts.go'             = 'coordination'
    'execution_attempts_test.go'        = 'coordination'
    'facebook_status.go'                = 'identities'
    'group_quality.go'                  = 'crawl'
    'groups.go'                         = 'crawl'
    'identities.go'                     = 'app'
    'knowledge_assets.go'               = 'knowledge'
    'knowledge_assets_test.go'          = 'knowledge'
    'knowledge_cost.go'                 = 'knowledge'
    'knowledge_embeddings.go'           = 'knowledge'
    'knowledge_embeddings_test.go'      = 'knowledge'
    'knowledge_events.go'               = 'knowledge'
    'knowledge_events_test.go'          = 'knowledge'
    'knowledge_feedback.go'             = 'knowledge'
    'knowledge_replay.go'               = 'knowledge'
    'knowledge_replay_test.go'          = 'knowledge'
    'knowledge_soak.go'                 = 'knowledge'
    'knowledge_soak_test.go'            = 'knowledge'
    'knowledge_sources.go'              = 'knowledge'
    'knowledge_sources_test.go'         = 'knowledge'
    'knowledge_vector_query.go'         = 'knowledge'
    'kpi.go'                            = 'app'
    'lead_engagement.go'                = 'leads'
    'lead_engagement_test.go'           = 'leads'
    'leads.go'                          = 'leads'
    'leads_repair_test.go'              = 'leads'
    'learning.go'                       = 'app'
    'media_assets.go'                   = 'app'
    'migrator.go'                       = 'infra'
    'organization.go'                   = 'users'
    'outbound.go'                       = 'outbound'
    'outbound_claim.go'                 = 'outbound'
    'outbound_dedup.go'                 = 'outbound'
    'outbound_edit.go'                  = 'outbound'
    'outbound_finalize.go'              = 'outbound'
    'outbound_lease.go'                 = 'outbound'
    'outbound_query.go'                 = 'outbound'
    'outbound_queue.go'                 = 'outbound'
    'outbound_queue_test.go'            = 'outbound'
    'outbound_transition.go'            = 'outbound'
    'outbound_transition_test.go'       = 'outbound'
    'postgres_driver.go'                = 'dbutil'
    'posts.go'                          = 'crawl'
    'price_items.go'                    = 'app'
    'private_files.go'                  = 'crawl'
    'prompt_memory.go'                  = 'prompts'
    'prompt_routing.go'                 = 'prompts'
    'prompt_routing_test.go'            = 'prompts'
    'schema.go'                         = 'infra'
    'schema_migrate_test.go'            = 'infra'
    'schema_template_test.go'           = 'infra'
    'selector_cache.go'                 = 'connectors'
    'session_status.go'                 = 'identities'
    'sessions.go'                       = 'identities'
    'skills.go'                         = 'prompts'
    'sqlite.go'                         = 'dbutil'
    'stats.go'                          = 'app'
    'store.go'                          = 'infra'
    'threads.go'                        = 'threads'
    'users.go'                          = 'users'
}

$repoRoot = (Get-Location).Path
$storeDir = Join-Path $repoRoot 'internal\store'
$changed = 0
$skipped = 0
$missing = @()

Get-ChildItem -Path $storeDir -File -Filter '*.go' | ForEach-Object {
    $name = $_.Name
    if (-not $mapping.ContainsKey($name)) {
        $missing += $name
        return
    }
    $content = Get-Content -Raw -Path $_.FullName
    if ($content -match '(?m)^// Domain:') {
        $skipped++
        return
    }
    # Find the package declaration line. Insert the Domain comment immediately above it,
    # preserving any existing package doc comment.
    $domain = $mapping[$name]
    $tag = "// Domain: $domain (see internal/store/DOMAINS.md)`r`n"
    # Insert before `package store` (or whatever the package decl is).
    $patched = $content -replace '(?m)^(package\s+\w+)', ($tag + '$1')
    if ($patched -ne $content) {
        Set-Content -Path $_.FullName -Value $patched -NoNewline
        $changed++
    }
}

Write-Output "tagged: $changed"
Write-Output "skipped (already tagged): $skipped"
if ($missing.Count -gt 0) {
    Write-Output "MISSING from mapping:"
    $missing | ForEach-Object { Write-Output "  $_" }
}
