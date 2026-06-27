package retrieval

// TruncateQuery caps a user-supplied query at the storage limit.
// Length is the SAME in every searcher's trace.Query field so the
// Replay UI never sees "embedding hit but query column truncated
// in retrieval-X format only" surprises.
const maxQueryStored = 240

func TruncateQuery(q string) string {
	if len(q) <= maxQueryStored {
		return q
	}
	return q[:maxQueryStored] + "…"
}
