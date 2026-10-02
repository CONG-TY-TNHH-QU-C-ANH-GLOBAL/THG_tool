package crmleadsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/thg/scraper/internal/models"
)

const DefaultShippingQuoteURL = "https://crm.thgfulfill.com/api/integrations/thg-tool/shipping-quote"

// QuoteSupplierShipment sends the post's observed quantity and destination to
// CRM's rate router. CRM alone selects a published lane and validates freshness.
func QuoteSupplierShipment(ctx context.Context, endpoint, key string, input models.ShippingRequest) (*models.ShippingReference, error) {
	if endpoint == "" || key == "" || input.Quantity <= 0 || input.WeightKG <= 0 || input.WeightKG > 20 || input.DestinationCountry == "" {
		return nil, errors.New("shipping quote not configured or missing shipment facts")
	}
	data, _ := json.Marshal(map[string]any{
		"originCountry": input.OriginCountry, "destinationCountry": input.DestinationCountry,
		"quantity": input.Quantity, "shipmentMode": input.ShipmentMode,
		"cargoCategory": input.CargoCategory, "weightKg": input.WeightKG,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-thg-integration-key", key)
	// A slow rate lookup must not erase the product suggestion.
	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CRM shipping quote HTTP %d", resp.StatusCode)
	}
	var quote struct {
		OK         bool    `json:"ok"`
		Lane       string  `json:"lane"`
		Currency   string  `json:"currency"`
		TotalUSD   float64 `json:"totalUsd"`
		BillableKG float64 `json:"billableKg"`
		Transit    string  `json:"transit"`
		Source     string  `json:"source"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&quote); err != nil {
		return nil, err
	}
	if !quote.OK || quote.Currency != "USD" || quote.TotalUSD <= 0 || quote.BillableKG <= 0 {
		return nil, errors.New("CRM has no usable shipping quote")
	}
	basis := fmt.Sprintf("%s %s→%s, 1 kiện %.3g kg; cước tham chiếu, chưa gồm phụ phí phát sinh", quote.Lane, input.OriginCountry, input.DestinationCountry, quote.BillableKG)
	if input.ShipmentMode == "bulk" {
		basis = fmt.Sprintf("%s %s→%s, 1 sản phẩm %.3g kg nếu gửi riêng; cước tham chiếu, không phải tổng cước lô %d sản phẩm", quote.Lane, input.OriginCountry, input.DestinationCountry, quote.BillableKG, input.Quantity)
	}
	return &models.ShippingReference{
		PriceText: fmt.Sprintf("$%.2f", quote.TotalUSD),
		Transit:   strings.TrimSpace(quote.Transit),
		Basis:     basis,
		SourceURL: strings.TrimSpace(quote.Source),
	}, nil
}
