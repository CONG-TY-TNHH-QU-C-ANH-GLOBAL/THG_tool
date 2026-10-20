package facebook

import "testing"

func TestKnowledgeReplyRejectsUnsupportedClaims(t *testing.T) {
	post := "Need a shipping agent for 200kg per month, 2 parcels, to the US"
	for _, reply := range []string{
		"We deliver to the US in 200 days.",
		"Delivery takes 2 working days.",
		"Giao hàng trong 2 giờ.",
		"Hàng đến sau 2 ngay.",
		"Delivery in 2–200 days.",
		"We offer free shipping to the US.",
		"Bên mình miễn phí vận chuyển.",
		"This product matches your photo.",
		"Mình đã đối chiếu ảnh và xác nhận đúng mẫu.",
	} {
		if safeKnowledgeReply(reply, post, LeadSuggestion{}) {
			t.Errorf("unsupported claim accepted: %q", reply)
		}
	}
	if !safeKnowledgeReply("For your 200kg monthly volume, we can check shipping options to the US. What are the parcel dimensions?", post, LeadSuggestion{}) {
		t.Fatal("known demand must remain usable without becoming a transit commitment")
	}
	if !safeKnowledgeReply("Could you share a product photo so we can check the model?", post, LeadSuggestion{}) {
		t.Fatal("asking for an image is different from claiming image verification")
	}
}
