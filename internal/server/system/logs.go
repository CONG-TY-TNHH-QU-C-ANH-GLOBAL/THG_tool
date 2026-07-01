package system

import (
	"bufio"
	"fmt"
	"sort"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/logstream"
	"github.com/thg/scraper/internal/store"
	"github.com/valyala/fasthttp"
)

// streamLogs serves Server-Sent Events with live log output.
// Uses ?token= query param for auth (EventSource cannot set headers).
func StreamLogs(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	ch := logstream.Global().Subscribe()

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		defer logstream.Global().Unsubscribe(ch)
		writeLogBackfill(w)
		streamLogEvents(w, ch)
	}))
	return nil
}

// writeLogBackfill flushes the last 100 buffered log lines to a freshly
// connected SSE client before live events start streaming.
func writeLogBackfill(w *bufio.Writer) {
	for _, e := range logstream.Global().Recent(100) {
		fmt.Fprint(w, e.SSEFormat())
	}
	_ = w.Flush()
}

// streamLogEvents blocks, writing each new log entry as an SSE frame and
// sending a keepalive ping on idle so proxies don't close the connection.
// Returns once the channel closes or a write fails.
func streamLogEvents(w *bufio.Writer, ch chan logstream.Entry) {
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprint(w, e.SSEFormat())
			if err := w.Flush(); err != nil {
				return
			}
		case <-time.After(25 * time.Second):
			fmt.Fprint(w, ": ping\n\n")
			if err := w.Flush(); err != nil {
				return
			}
		}
	}
}

// getSentimentStats returns lead analytics: score breakdown, niche distribution, outbound status.
func SentimentStats(db *store.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		stats, _ := db.App().GetStats()

		// Sample up to 500 leads to compute niche distribution and score breakdown
		leads, _ := db.Leads().GetLeadsFiltered("", "", 500, 0, 0)

		nicheCounts := make(map[string]int)
		scoreCounts := map[string]int{"hot": 0, "warm": 0, "cold": 0, "rejected": 0}
		for _, l := range leads {
			if l.Niche != "" {
				nicheCounts[l.Niche]++
			}
			scoreCounts[string(l.Score)]++
		}

		type nicheEntry struct {
			Niche string `json:"niche"`
			Count int    `json:"count"`
		}
		var topNiches []nicheEntry
		for k, v := range nicheCounts {
			topNiches = append(topNiches, nicheEntry{k, v})
		}
		sort.Slice(topNiches, func(i, j int) bool { return topNiches[i].Count > topNiches[j].Count })
		if len(topNiches) > 10 {
			topNiches = topNiches[:10]
		}

		orgID, _ := c.Locals("org_id").(int64)
		outboundCounts, _ := db.Outbound().CountByState(orgID)

		return c.JSON(fiber.Map{
			"stats":           stats,
			"score_breakdown": scoreCounts,
			"top_niches":      topNiches,
			"outbound":        outboundCounts,
		})
	}
}
