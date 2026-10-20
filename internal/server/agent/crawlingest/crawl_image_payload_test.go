package crawlingest

import (
	"encoding/json"
	"testing"
)

func TestCrawlItemImageURLsAreAdditive(t *testing.T) {
	for _, payload := range []string{
		`{"content":"Need this product","image_urls":["https://scontent.fbcdn.net/post.jpg"]}`,
		`{"content":"Need this product"}`,
	} {
		var item connectorCrawlItem
		if err := json.Unmarshal([]byte(payload), &item); err != nil || item.Content != "Need this product" {
			t.Fatalf("crawl item contract broke: %v", err)
		}
	}
	var item connectorCrawlItem
	_ = json.Unmarshal([]byte(`{"image_urls":["https://scontent.fbcdn.net/post.jpg"]}`), &item)
	if len(item.ImageURLs) != 1 {
		t.Fatal("image references were silently discarded during JSON decoding")
	}
}
