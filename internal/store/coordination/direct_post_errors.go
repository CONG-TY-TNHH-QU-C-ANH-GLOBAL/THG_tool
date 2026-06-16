package coordination

// Typed terminal error codes for the direct-post intake CONTENT/CONTEXT guards (P1.3).
// They are distinct from DPErrIdentityMismatch (P1.1, post-id/group identity collision):
// these fire when the requested post is positively identified but the imported item or
// the stored lead carries wrong-context/garbage content, so no comment must be sent.
const (
	// DPErrImportedItemContextMismatch — the import-result item positively matched the
	// requested post id but its group/author context or content was invalid, so no lead
	// was created (ingest-time guard, P1.3A).
	DPErrImportedItemContextMismatch = "imported_item_context_mismatch"

	// DPErrLeadContentInvalid — the imported item had no usable post text (UI chrome /
	// boilerplate), so no lead was created (ingest-time guard, P1.3A).
	DPErrLeadContentInvalid = "lead_content_invalid"

	// DPErrLeadTargetMismatch — a strict-canonical-matched lead exists in the DB but its
	// content/context conflicts with the requested target, so the comment is refused at
	// queue time (pre-comment guard, P1.3B). Matches the manual quarantine reason code.
	DPErrLeadTargetMismatch = "lead_target_context_mismatch"
)

// Granular direct-post IMPORT outcome codes (P1.3C). They replace the generic
// "awaiting single-post import" / lead_not_observed loop with a precise reason an operator
// can act on. The ingest path (crawl-result) sets the *_rejected/*_mismatch/*_content/
// *_no_observed_item codes when the connector task finishes; the poller still owns the
// timeout terminal DPErrLeadNotObserved when no result ever arrives.
const (
	// DPErrImportNoObservedItem — the connector import task FINISHED but produced no valid
	// observed item for the requested post (empty result, or only neighbour/foreign items).
	// Re-observing cannot help, so the workflow fails deterministically instead of looping.
	DPErrImportNoObservedItem = "direct_post_import_no_observed_item"
	// DPErrImportNoMeaningfulContent — the requested post was observed but its content was
	// empty/UI-chrome/boilerplate (no usable post text).
	DPErrImportNoMeaningfulContent = "direct_post_import_no_meaningful_content"
	// DPErrImportRejectedByGuard — the observed item positively matched the requested post
	// but was rejected by the zero-trust guard (generic fallback when a more specific code
	// does not apply).
	DPErrImportRejectedByGuard = "direct_post_import_rejected_by_guard"
	// DPErrImportPostIDMissing — the observed item carried no positively-verifiable post id.
	DPErrImportPostIDMissing = "direct_post_import_post_id_missing"
	// DPErrImportPostIDMismatch — the observed item's post id differed from the requested.
	DPErrImportPostIDMismatch = "direct_post_import_post_id_mismatch"
	// DPErrImportGroupMismatch — the observed item's group/author context conflicted with
	// the requested group (the wrong-content incident class).
	DPErrImportGroupMismatch = "direct_post_import_group_mismatch"
	// DPErrImportAccountMismatch — the import ran on a different account than the action
	// account (should not happen with P1.3C pinning; reserved for detection/diagnostics).
	DPErrImportAccountMismatch = "direct_post_import_account_mismatch"
	// DPErrImportBoilerplateContent — the requested post's content was boilerplate/UI chrome.
	DPErrImportBoilerplateContent = "direct_post_import_boilerplate_content"
)
