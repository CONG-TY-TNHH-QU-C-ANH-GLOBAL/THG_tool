package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
)

// PriceExtractor uses OpenAI Vision to extract pricing data from images or text.
type PriceExtractor struct {
	apiKey string
	model  string
	client *http.Client
}

// NewPriceExtractor creates a new price extractor.
func NewPriceExtractor(apiKey, model string) *PriceExtractor {
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &PriceExtractor{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// Available returns true if configured.
func (pe *PriceExtractor) Available() bool { return pe.apiKey != "" }

// ExtractFromImage reads a local image file and extracts pricing info using OpenAI Vision.
func (pe *PriceExtractor) ExtractFromImage(ctx context.Context, imagePath string) ([]models.PriceItem, error) {
	data, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}

	// Detect MIME type
	mime := "image/jpeg"
	if strings.HasSuffix(strings.ToLower(imagePath), ".png") {
		mime = "image/png"
	} else if strings.HasSuffix(strings.ToLower(imagePath), ".webp") {
		mime = "image/webp"
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mime, b64)

	prompt := `Đây là bảng giá dịch vụ/sản phẩm. Hãy trích xuất tất cả thông tin giá cả.

Trả về JSON với format sau (CHÍNH XÁC):
{"items": [{"service_name": "tên dịch vụ/sản phẩm", "price": "mức giá", "unit": "đơn vị (kg/lb/tháng/cái/...)", "notes": "ghi chú thêm nếu có"}]}

Nếu không tìm thấy bảng giá, trả về: {"items": []}`

	body := map[string]any{
		"model": pe.model,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "text", "text": prompt},
					{"type": "image_url", "image_url": map[string]string{"url": dataURL}},
				},
			},
		},
		"response_format": map[string]string{"type": "json_object"},
		"max_tokens":      1000,
	}

	return pe.callAndParse(ctx, body)
}

// ExtractFromText parses a free-form text price list into structured items.
func (pe *PriceExtractor) ExtractFromText(ctx context.Context, text string) ([]models.PriceItem, error) {
	prompt := fmt.Sprintf(`Đây là thông tin bảng giá dịch vụ/sản phẩm:

"%s"

Hãy trích xuất thành JSON với format (CHÍNH XÁC):
{"items": [{"service_name": "tên dịch vụ/sản phẩm", "price": "mức giá", "unit": "đơn vị", "notes": "ghi chú nếu có"}]}

Nếu text không phải bảng giá, trả về: {"items": []}`, text)

	body := map[string]any{
		"model": pe.model,
		"messages": []map[string]any{
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
		"max_tokens":      800,
	}

	return pe.callAndParse(ctx, body)
}

func (pe *PriceExtractor) callAndParse(ctx context.Context, body map[string]any) ([]models.PriceItem, error) {
	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+pe.apiKey)

	resp, err := pe.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI HTTP %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Choices []struct {
			Message struct{ Content string } `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no response")
	}

	var parsed struct {
		Items []struct {
			ServiceName string `json:"service_name"`
			Price       string `json:"price"`
			Unit        string `json:"unit"`
			Notes       string `json:"notes"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(result.Choices[0].Message.Content), &parsed); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	var items []models.PriceItem
	for _, p := range parsed.Items {
		if p.ServiceName == "" || p.Price == "" {
			continue
		}
		items = append(items, models.PriceItem{
			ServiceName: p.ServiceName,
			Price:       p.Price,
			Unit:        p.Unit,
			Notes:       p.Notes,
		})
	}
	return items, nil
}

// FormatPriceList formats price items into a human-readable string for AI prompts.
func FormatPriceList(items []models.PriceItem) string {
	if len(items) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("BẢNG GIÁ DỊCH VỤ:\n")
	for _, item := range items {
		line := fmt.Sprintf("- %s: %s", item.ServiceName, item.Price)
		if item.Unit != "" {
			line += "/" + item.Unit
		}
		if item.Notes != "" {
			line += " (" + item.Notes + ")"
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}
