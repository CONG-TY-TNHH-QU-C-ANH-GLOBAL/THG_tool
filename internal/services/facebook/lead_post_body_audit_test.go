package facebook

import "testing"

func TestLeadPostBodyRemovesFollowAndRecentHeaders(t *testing.T) {
	const body = "Need 200 custom mugs shipped to Canada"
	for _, header := range []string{
		"Facebook Sample Author · Theo dõi 9 phút trước · ",
		"Facebook Sample Author · Follow · 2 hrs ago · ",
		"Facebook Sample Author · Theo dõi · Quản trị viên · 2 giờ trước · ",
		"Facebook Sample Author Vừa xong · ",
		"Facebook Sample Author · Just now · ",
	} {
		t.Run(header, func(t *testing.T) {
			if got := leadPostBody(header + body + " Xem bản dịch 3 2"); got != body {
				t.Fatalf("header leaked into model input: %q", got)
			}
		})
	}
}
