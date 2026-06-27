package embedding

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func mkResp(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

// embedHTTPStatusError must preserve the recoverable/permanent classification the
// worker's retry policy depends on: 5xx + 429 recoverable, other 4xx permanent,
// 2xx acceptable (nil).
func TestEmbedHTTPStatusError(t *testing.T) {
	if err := embedHTTPStatusError(mkResp(200, "")); err != nil {
		t.Fatalf("200 should classify as nil, got %v", err)
	}
	if err := embedHTTPStatusError(mkResp(503, "")); err == nil || !IsRecoverable(err) {
		t.Fatalf("5xx should be recoverable, got %v", err)
	}
	if err := embedHTTPStatusError(mkResp(429, "")); err == nil || !IsRecoverable(err) {
		t.Fatalf("429 should be recoverable, got %v", err)
	}
	if err := embedHTTPStatusError(mkResp(400, "bad request")); err == nil || !IsPermanent(err) {
		t.Fatalf("other 4xx should be permanent, got %v", err)
	}
}

// parseEmbeddingVectors must order vectors by Index (OpenAI gives no order
// guarantee) and reject count/dimension mismatches as permanent.
func TestParseEmbeddingVectors(t *testing.T) {
	body := `{"data":[{"index":1,"embedding":[0.1,0.2]},{"index":0,"embedding":[0.3,0.4]}]}`
	out, err := parseEmbeddingVectors(mkResp(200, body), 2, 2)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if out[0][0] != 0.3 || out[1][0] != 0.1 {
		t.Fatalf("vectors must be ordered by index, got %v", out)
	}

	if _, err := parseEmbeddingVectors(mkResp(200, `{"data":[{"index":0,"embedding":[0.1,0.2]}]}`), 2, 2); err == nil || !IsPermanent(err) {
		t.Fatalf("count mismatch should be permanent, got %v", err)
	}
	if _, err := parseEmbeddingVectors(mkResp(200, `{"data":[{"index":0,"embedding":[0.1]}]}`), 1, 2); err == nil || !IsPermanent(err) {
		t.Fatalf("dimension mismatch should be permanent, got %v", err)
	}
	if _, err := parseEmbeddingVectors(mkResp(200, `{bad json`), 1, 2); err == nil || !IsRecoverable(err) {
		t.Fatalf("decode error should be recoverable, got %v", err)
	}
}
