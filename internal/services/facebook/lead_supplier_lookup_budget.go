package facebook

import (
	"context"
	"errors"
	"time"
)

var errSupplierBudget = errors.New("insufficient time for another marketplace request")

// Pricing Hub calls can each take four seconds. Leave time for one detail and
// the CRM shipping lookup before the lead notification deadline.
func supplierLookupBudget(ctx context.Context, minimum time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < minimum {
		return errSupplierBudget
	}
	return nil
}
