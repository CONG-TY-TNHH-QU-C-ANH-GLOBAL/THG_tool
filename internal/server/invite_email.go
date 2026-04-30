package server

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/thg/scraper/internal/mailer"
)

type inviteEmailResult struct {
	Status string
	Error  string
	URL    string
}

func (s *Server) sendWorkspaceInviteEmail(c *fiber.Ctx, inviteID, orgID, actorID int64, email, role, token, expiresAt string) inviteEmailResult {
	inviteURL := s.publicInviteURL(c, token)
	result := inviteEmailResult{Status: "not_configured", URL: inviteURL}
	if !s.cfg.Mailer.Enabled() {
		_ = s.updateInviteEmailStatus(inviteID, result.Status, "")
		return result
	}

	orgName := "THG AutoFlow"
	if org, err := s.db.GetOrganization(orgID); err == nil && org != nil && strings.TrimSpace(org.Name) != "" {
		orgName = org.Name
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := mailer.SendInvite(ctx, s.cfg.Mailer, mailer.InviteMessage{
		ToEmail:   email,
		OrgName:   orgName,
		Role:      role,
		InviteURL: inviteURL,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		_ = s.updateInviteEmailStatus(inviteID, result.Status, result.Error)
		_ = s.db.InsertAuditLog(actorID, "workspace_invite_email_failed", c.IP(),
			fmt.Sprintf(`{"invite_id":%d,"org_id":%d,"email":%q,"error":%q}`, inviteID, orgID, email, result.Error))
		log.Printf("[InviteEmail] failed invite_id=%d email=%s: %v", inviteID, email, err)
		return result
	}

	result.Status = "sent"
	_ = s.updateInviteEmailStatus(inviteID, result.Status, "")
	_ = s.db.InsertAuditLog(actorID, "workspace_invite_email_sent", c.IP(),
		fmt.Sprintf(`{"invite_id":%d,"org_id":%d,"email":%q}`, inviteID, orgID, email))
	return result
}

func (s *Server) updateInviteEmailStatus(inviteID int64, status, errText string) error {
	if inviteID <= 0 {
		return nil
	}
	_, err := s.db.DB().Exec(`
		UPDATE org_invites
		SET email_status = ?,
		    email_error = ?,
		    email_sent_at = CASE WHEN ? = 'sent' THEN CURRENT_TIMESTAMP ELSE email_sent_at END
		WHERE id = ?`,
		status, errText, status, inviteID)
	return err
}

func (s *Server) publicInviteURL(c *fiber.Ctx, token string) string {
	base := strings.TrimRight(strings.TrimSpace(s.cfg.Mailer.AppBaseURL), "/")
	if base == "" {
		base = requestPublicBaseURL(c)
	}
	if base == "" {
		base = "/"
	}
	if strings.HasSuffix(base, "/") {
		return base + "join/" + token
	}
	return base + "/join/" + token
}

func requestPublicBaseURL(c *fiber.Ctx) string {
	proto := strings.TrimSpace(c.Get("X-Forwarded-Proto"))
	host := strings.TrimSpace(c.Get("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(c.Get("Host"))
	}
	if proto == "" {
		if origin := strings.TrimSpace(c.Get("Origin")); origin != "" {
			if u, err := url.Parse(origin); err == nil && u.Scheme != "" && u.Host != "" {
				return u.Scheme + "://" + u.Host
			}
		}
		if referer := strings.TrimSpace(c.Get("Referer")); referer != "" {
			if u, err := url.Parse(referer); err == nil && u.Scheme != "" && u.Host != "" {
				return u.Scheme + "://" + u.Host
			}
		}
		proto = c.Protocol()
	}
	if host == "" {
		return ""
	}
	return proto + "://" + host
}
