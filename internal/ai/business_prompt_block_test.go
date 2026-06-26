package ai

import "testing"

// TestBusinessProfile_ToPromptBlock pins the exact prompt block (label order +
// "LABEL: value\n" shape) because it is injected verbatim into classifier and
// comment-generation prompts. Characterization for the go:S3776 refactor.
func TestBusinessProfile_ToPromptBlock(t *testing.T) {
	if got := (&BusinessProfile{}).ToPromptBlock(); got != "(Business profile not configured — user should describe their business first)" {
		t.Fatalf("unconfigured block mismatch: %q", got)
	}

	p := &BusinessProfile{
		Name: "Acme", Industry: "POD", Description: "we print", Services: "tees",
		Targets: "shops", TargetAuthorRole: "buyer", TargetSignals: "want bulk",
		NegativeSignals: "spam", Location: "VN", Markets: "US", USP: "fast",
		Tone: "warm", ApprovalPolicy: "manual", RejectRules: "no scam",
	}
	want := "BUSINESS: Acme\n" +
		"INDUSTRY: POD\n" +
		"WHAT WE DO: we print\n" +
		"PRODUCTS/SERVICES: tees\n" +
		"IDEAL CUSTOMER: shops\n" +
		"TARGET AUTHOR ROLE: buyer\n" +
		"PREFER POSTS WITH THESE SIGNALS: want bulk\n" +
		"REJECT POSTS WITH THESE SIGNALS: spam\n" +
		"LOCATION: VN\n" +
		"TARGET MARKETS: US\n" +
		"WHY CHOOSE US: fast\n" +
		"TONE: warm\n" +
		"APPROVAL POLICY: manual\n" +
		"IGNORE THESE POSTS: no scam\n"
	if got := p.ToPromptBlock(); got != want {
		t.Fatalf("block mismatch:\n got %q\nwant %q", got, want)
	}

	// A blank field is omitted entirely (no empty "LABEL: \n" line).
	if got := (&BusinessProfile{Industry: "POD"}).ToPromptBlock(); got != "INDUSTRY: POD\n" {
		t.Fatalf("single-field block mismatch: %q", got)
	}
}
