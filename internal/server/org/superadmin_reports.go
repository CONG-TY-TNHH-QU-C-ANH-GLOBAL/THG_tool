package org

import (
	"context"
	"database/sql"
	"errors"
)

// Superadmin diagnostic reports.
//
// The POST /superadmin/query endpoint executes ONLY the fixed, compile-time
// SELECT constants below, selected by an allowlisted report key — never
// request-provided SQL. This keeps the founder diagnostic console from being
// used to run arbitrary queries. Each query selects an explicit, non-sensitive
// column set (no password_hash) and is bounded by a fixed LIMIT.

// Allowlisted report keys accepted in the request body.
const (
	superadminReportOrganizations = "organizations"
	superadminReportUsers         = "users"
)

// errUnknownReport is returned when a requested report key is not allowlisted.
var errUnknownReport = errors.New("unknown report")

const superadminOrganizationsReportSQL = `SELECT id, name, domain, plan_tier, max_accounts, active, created_at
FROM organizations
ORDER BY id
LIMIT 100`

const superadminUsersReportSQL = `SELECT id, email, name, role, active, org_id, created_at
FROM users
ORDER BY id
LIMIT 100`

// querySuperadminReport runs the fixed report query for an allowlisted key.
// Every branch passes a compile-time SQL constant directly to the driver;
// an unknown key returns errUnknownReport without touching the database.
func (h *Handler) querySuperadminReport(ctx context.Context, report string) (*sql.Rows, error) {
	switch report {
	case superadminReportOrganizations:
		return h.deps.DB.DB().QueryContext(ctx, superadminOrganizationsReportSQL)
	case superadminReportUsers:
		return h.deps.DB.DB().QueryContext(ctx, superadminUsersReportSQL)
	default:
		return nil, errUnknownReport
	}
}

// scanSuperadminReportRows reads a report result set into column names and a
// capped list of row maps, preserving the endpoint's existing response shape.
func scanSuperadminReportRows(rows *sql.Rows) ([]string, []map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	var result []map[string]any
	for rows.Next() {
		ptrs := make([]any, len(cols))
		vals := make([]any, len(cols))
		for i := range ptrs {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, err
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
	return cols, result, nil
}
