package org

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func (h *Handler) superAdminAccounts(c *fiber.Ctx) error {
	accounts, err := h.deps.DB.GetAllAccounts(0)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"accounts": accounts, "count": len(accounts)})
}

func (h *Handler) superAdminUsers(c *fiber.Ctx) error {
	rows, err := h.deps.DB.DB().Query(
		`SELECT id, COALESCE(org_id,0), name, email, role, COALESCE(active,0), COALESCE(created_at,'')
		 FROM users ORDER BY created_at DESC`,
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	type userRow struct {
		ID        int64  `json:"id"`
		OrgID     int64  `json:"org_id"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		Role      string `json:"role"`
		Active    int    `json:"active"`
		CreatedAt string `json:"created_at"`
	}
	var users []userRow
	for rows.Next() {
		var u userRow
		if err := rows.Scan(&u.ID, &u.OrgID, &u.Name, &u.Email, &u.Role, &u.Active, &u.CreatedAt); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		users = append(users, u)
	}
	return c.JSON(fiber.Map{"users": users, "count": len(users)})
}

func (h *Handler) superAdminSessions(c *fiber.Ctx) error {
	rows, err := h.deps.DB.DB().Query(
		`SELECT account_id, COALESCE(org_id,0), status,
		        COALESCE(cdp_port,0), COALESCE(vnc_port,0),
		        COALESCE(started_at,''), COALESCE(last_active_at,'')
		 FROM browser_sessions WHERE status != 'terminated'
		 ORDER BY started_at DESC`,
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	type sessionRow struct {
		AccountID    int64  `json:"account_id"`
		OrgID        int64  `json:"org_id"`
		Status       string `json:"status"`
		CDPPort      int64  `json:"cdp_port"`
		VNCPort      int64  `json:"vnc_port"`
		StartedAt    string `json:"started_at"`
		LastActiveAt string `json:"last_active_at"`
	}
	var sessions []sessionRow
	for rows.Next() {
		var ss sessionRow
		if err := rows.Scan(&ss.AccountID, &ss.OrgID, &ss.Status, &ss.CDPPort, &ss.VNCPort, &ss.StartedAt, &ss.LastActiveAt); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		sessions = append(sessions, ss)
	}
	return c.JSON(fiber.Map{"sessions": sessions, "count": len(sessions)})
}

func (h *Handler) superAdminDeleteOrg(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if id == 1 {
		return c.Status(403).JSON(fiber.Map{"error": "cannot delete platform org"})
	}
	if _, err := h.deps.DB.DB().ExecContext(c.Context(), `DELETE FROM organizations WHERE id = ?`, id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *Handler) superAdminDeleteAccount(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if h.deps.Workspace != nil {
		h.deps.Workspace.Stop(id)
	}
	if err := h.deps.DB.DeleteAccount(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *Handler) superAdminDeleteUser(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	// Prevent self-delete
	if selfID, _ := c.Locals("user_id").(int64); selfID == id {
		return c.Status(403).JSON(fiber.Map{"error": "cannot delete your own account"})
	}
	if err := h.deps.DB.DeleteUser(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *Handler) superAdminTerminateSession(c *fiber.Ctx) error {
	accountID, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if h.deps.Workspace != nil {
		h.deps.Workspace.Stop(accountID)
	}
	_, err = h.deps.DB.DB().ExecContext(c.Context(),
		`UPDATE browser_sessions SET status = 'terminated' WHERE account_id = ?`, accountID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *Handler) superAdminQuery(c *fiber.Ctx) error {
	var body struct {
		SQL string `json:"sql"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	query := strings.TrimSpace(body.SQL)
	if query == "" || !strings.HasPrefix(strings.ToUpper(query), "SELECT") {
		return c.Status(403).JSON(fiber.Map{"error": "only SELECT queries allowed"})
	}
	rows, err := h.deps.DB.DB().QueryContext(c.Context(), query)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	var result []map[string]any
	for rows.Next() {
		ptrs := make([]any, len(cols))
		vals := make([]any, len(cols))
		for i := range ptrs {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			row[col] = vals[i]
		}
		result = append(result, row)
		if len(result) >= 500 {
			break
		}
	}
	return c.JSON(fiber.Map{"columns": cols, "rows": result, "count": len(result)})
}
