package facebook

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/thg/scraper/internal/models"
)

func TestLeadSuggestionLogOmitsLeadContent(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	LogLeadSuggestionOutcome("crawl", 7, SuggestionGateAttempted, models.LeadSuggestion{
		Reply: "Hi Secret Person, call 0909123456", SourcingNote: noOfferSourcingNote,
	})
	got := buf.String()
	for _, want := range []string{"path=crawl", "org_id=7", "gate=attempted", "reply=true", "supplier=false"} {
		if !strings.Contains(got, want) {
			t.Errorf("log %q lacks %q", got, want)
		}
	}
	if strings.Contains(got, "Secret Person") || strings.Contains(got, "0909") {
		t.Fatalf("log must not contain reply text or contact data: %q", got)
	}
}
