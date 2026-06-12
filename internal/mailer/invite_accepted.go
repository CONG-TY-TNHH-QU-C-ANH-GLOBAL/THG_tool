package mailer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
)

// SendInviteAccepted notifies the inviter/admin that their invite was
// accepted (SaaS UX Hardening PR-8). Plain-text, best-effort — callers
// fire it async exactly like the invite email.
func SendInviteAccepted(ctx context.Context, cfg Config, toEmail, orgName, memberName, memberEmail, role string) error {
	cfg = cfg.normalized()
	if !cfg.Enabled() {
		return errors.New("smtp is not configured")
	}
	toEmail = strings.TrimSpace(toEmail)
	if toEmail == "" {
		return errors.New("recipient is required")
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	from := mail.Address{Name: cfg.FromName, Address: cfg.FromEmail}
	to := mail.Address{Address: toEmail}
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", from.String())
	fmt.Fprintf(&b, "To: %s\r\n", to.String())
	fmt.Fprintf(&b, "Subject: %s joined %s on THG AutoFlow\r\n", memberName, orgName)
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n")
	fmt.Fprintf(&b, "%s (%s) accepted your invite and joined workspace %s as %s.\r\n",
		memberName, memberEmail, orgName, role)
	return sendSMTP(ctx, cfg, toEmail, b.Bytes())
}
