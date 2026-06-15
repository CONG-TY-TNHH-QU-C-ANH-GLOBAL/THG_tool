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
