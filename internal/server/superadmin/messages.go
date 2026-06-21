package superadmin

// HTTP error-response message strings used by the superadmin handlers. These
// are intentionally duplicated wire-format literals (not imported from org) so
// this package stays decoupled from the org module; values are byte-identical
// to the strings these handlers returned before the extraction.
const (
	msgInvalidID      = "invalid id"
	msgInvalidRequest = "invalid request"
)
