package crawlingest

import "strconv"

// itoa64 builds int64 JSON fragments in the crawl-result test bodies. Copied
// from the agent package's test helper when this cluster moved out — a one-liner
// not worth an exported shared-test package.
func itoa64(v int64) string {
	return strconv.FormatInt(v, 10)
}
