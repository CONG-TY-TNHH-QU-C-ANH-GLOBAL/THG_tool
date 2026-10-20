package visualcontext

import "testing"

func TestParseRequiresCompleteTentativeEvidence(t *testing.T) {
	valid := `{"product":"hand massager","observations":["Visible handheld device"],"uncertainties":["Model label unreadable"],"text_image_conflict":false}`
	got, err := Parse([]byte(valid))
	if err != nil || got.Product != "hand massager" || got.Conflict {
		t.Fatalf("valid context rejected: %+v, %v", got, err)
	}
	for _, invalid := range []string{
		`{"product":"hand massager"}`,
		`{"product":"hand massager","observations":[],"uncertainties":[],"text_image_conflict":false}`,
		`{"product":"ABC123","observations":[],"uncertainties":[],"text_image_conflict":false}`,
		`{"product":"mug","observations":["https://supplier.example"],"uncertainties":[],"text_image_conflict":false}`,
		`{"product":"mug","observations":null,"uncertainties":[],"text_image_conflict":false}`,
		`{"product":"mug","observations":[],"uncertainties":[],"text_image_conflict":null}`,
	} {
		if _, err := Parse([]byte(invalid)); err == nil {
			t.Errorf("invalid context accepted: %s", invalid)
		}
	}
}
