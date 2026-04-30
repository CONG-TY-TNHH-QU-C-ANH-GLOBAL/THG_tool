package mailer

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"html"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host               string
	Port               int
	Username           string
	Password           string
	FromEmail          string
	FromName           string
	AppBaseURL         string
	UseTLS             bool
	UseStartTLS        bool
	InsecureSkipVerify bool
	Timeout            time.Duration
}

type InviteMessage struct {
	ToEmail   string
	OrgName   string
	Role      string
	InviteURL string
	ExpiresAt string
}

func (c Config) Enabled() bool {
	return strings.TrimSpace(c.Host) != "" && strings.TrimSpace(c.FromEmail) != ""
}

func (c Config) normalized() Config {
	c.Host = strings.TrimSpace(c.Host)
	c.Username = strings.TrimSpace(c.Username)
	c.FromEmail = strings.TrimSpace(c.FromEmail)
	c.FromName = strings.TrimSpace(c.FromName)
	c.AppBaseURL = strings.TrimRight(strings.TrimSpace(c.AppBaseURL), "/")
	if c.Port == 0 {
		c.Port = 587
	}
	if c.Timeout <= 0 {
		c.Timeout = 10 * time.Second
	}
	return c
}

func SendInvite(ctx context.Context, cfg Config, msg InviteMessage) error {
	cfg = cfg.normalized()
	if !cfg.Enabled() {
		return errors.New("smtp is not configured")
	}
	if strings.TrimSpace(msg.ToEmail) == "" || strings.TrimSpace(msg.InviteURL) == "" {
		return errors.New("recipient and invite url are required")
	}

	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	raw, err := buildInviteEmail(cfg, msg)
	if err != nil {
		return err
	}
	return sendSMTP(ctx, cfg, msg.ToEmail, raw)
}

func buildInviteEmail(cfg Config, msg InviteMessage) ([]byte, error) {
	from := mail.Address{Name: cfg.FromName, Address: cfg.FromEmail}
	if cfg.FromName == "" {
		from = mail.Address{Address: cfg.FromEmail}
	}
	to := mail.Address{Address: strings.TrimSpace(msg.ToEmail)}
	subject := "You are invited to THG AutoFlow"

	var body bytes.Buffer
	body.WriteString(`<div style="font-family:Arial,sans-serif;background:#0d101a;color:#e5e7eb;padding:28px">`)
	body.WriteString(`<div style="max-width:560px;margin:auto;background:#111520;border:1px solid #242a3c;border-radius:14px;padding:28px">`)
	body.WriteString(`<h1 style="font-size:22px;margin:0 0 12px;color:#fff">Workspace invitation</h1>`)
	body.WriteString(`<p style="line-height:1.6;color:#cbd5e1">You have been invited to join <strong>`)
	body.WriteString(html.EscapeString(msg.OrgName))
	body.WriteString(`</strong> on THG AutoFlow as <strong>`)
	body.WriteString(html.EscapeString(msg.Role))
	body.WriteString(`</strong>.</p>`)
	body.WriteString(`<p style="line-height:1.6;color:#cbd5e1">Use the secure invite link below with this email address: <strong>`)
	body.WriteString(html.EscapeString(msg.ToEmail))
	body.WriteString(`</strong>.</p>`)
	body.WriteString(`<p style="margin:26px 0"><a href="`)
	body.WriteString(html.EscapeString(msg.InviteURL))
	body.WriteString(`" style="background:#4f46e5;color:#fff;text-decoration:none;padding:12px 18px;border-radius:10px;font-weight:700;display:inline-block">Join workspace</a></p>`)
	body.WriteString(`<p style="font-size:13px;line-height:1.6;color:#9ca3af">If the button does not work, copy this link:</p>`)
	body.WriteString(`<p style="font-size:12px;line-height:1.6;color:#a5b4fc;word-break:break-all">`)
	body.WriteString(html.EscapeString(msg.InviteURL))
	body.WriteString(`</p>`)
	if msg.ExpiresAt != "" {
		body.WriteString(`<p style="font-size:12px;color:#9ca3af">This invite expires at `)
		body.WriteString(html.EscapeString(msg.ExpiresAt))
		body.WriteString(`.</p>`)
	}
	body.WriteString(`</div></div>`)

	var out bytes.Buffer
	headers := map[string]string{
		"From":                      from.String(),
		"To":                        to.String(),
		"Subject":                   mime.QEncoding.Encode("UTF-8", subject),
		"MIME-Version":              "1.0",
		"Content-Type":              `text/html; charset="UTF-8"`,
		"Content-Transfer-Encoding": "quoted-printable",
		"Date":                      time.Now().Format(time.RFC1123Z),
	}
	for _, key := range []string{"From", "To", "Subject", "MIME-Version", "Content-Type", "Content-Transfer-Encoding", "Date"} {
		out.WriteString(key)
		out.WriteString(": ")
		out.WriteString(headers[key])
		out.WriteString("\r\n")
	}
	out.WriteString("\r\n")
	qp := quotedprintable.NewWriter(&out)
	if _, err := qp.Write(body.Bytes()); err != nil {
		return nil, err
	}
	if err := qp.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func sendSMTP(ctx context.Context, cfg Config, to string, raw []byte) error {
	address := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	dialer := &net.Dialer{Timeout: cfg.Timeout}
	tlsConfig := &tls.Config{
		ServerName:         cfg.Host,
		InsecureSkipVerify: cfg.InsecureSkipVerify, //nolint:gosec // explicit env-controlled escape hatch for private SMTP.
		MinVersion:         tls.VersionTLS12,
	}

	var (
		conn net.Conn
		err  error
	)
	if cfg.UseTLS || cfg.Port == 465 {
		conn, err = tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(cfg.Timeout))

	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if !(cfg.UseTLS || cfg.Port == 465) && cfg.UseStartTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("smtp starttls: %w", err)
			}
		}
	}

	if cfg.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(cfg.FromEmail); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := client.Rcpt(strings.TrimSpace(to)); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("smtp quit: %w", err)
	}
	return nil
}
