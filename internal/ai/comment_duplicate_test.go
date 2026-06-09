package ai

import "testing"

func TestDetectRepeatedText(t *testing.T) {
	const A = "Bên THG Fulfill có hỗ trợ sourcing Tumbler 20oz. Nếu cần, inbox THG Fulfill nhé."

	dup := map[string]string{
		"A+A":          A + A,
		"A + space + A": A + " " + A,
		"A + newline + A": A + "\n" + A,
	}
	for name, in := range dup {
		if !DetectRepeatedText(in) {
			t.Errorf("%s: expected duplicate detected", name)
		}
	}

	clean := map[string]string{
		"single":        A,
		"too short":     "ngắn",
		"distinct text": "Bên mình làm tumbler. Còn shop kia làm nailbox. Hai dịch vụ khác nhau hoàn toàn.",
	}
	for name, in := range clean {
		if DetectRepeatedText(in) {
			t.Errorf("%s: expected NOT duplicate", name)
		}
	}
}
