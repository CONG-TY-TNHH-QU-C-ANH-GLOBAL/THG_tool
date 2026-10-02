package suppliersourcing

import (
	"errors"
	"fmt"
	"strings"
)

var ErrQuotaExceeded = errors.New("Elim quota exhausted")

func scrapeRejection(endpoint, message string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "unknown upstream error"
	}
	lower := strings.ToLower(message)
	if strings.Contains(lower, "hết lượt") || strings.Contains(lower, "quota") || strings.Contains(lower, "credit") || strings.Contains(lower, "402") {
		return fmt.Errorf("suppliersourcing: %s rejected: %w: %s", endpoint, ErrQuotaExceeded, message)
	}
	return fmt.Errorf("suppliersourcing: %s rejected: %s", endpoint, message)
}
