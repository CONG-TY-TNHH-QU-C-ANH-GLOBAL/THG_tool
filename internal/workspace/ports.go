package workspace

import (
	"context"
	"database/sql"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

const (
	CDPPortBase  = 9300 // host ports 9300..9499 for CDP (supports 200 accounts)
	VNCPortBase  = 5910 // host ports 5910..6109 for VNC
	portRangeMax = 200
)

// PortRegistry tracks which host ports are allocated to which accounts.
// Persists assignments in the port_registry table and recovers them on restart.
type PortRegistry struct {
	mu  sync.Mutex
	cdp map[int]int64 // port → accountID; 0 = free
	vnc map[int]int64
	db  *sql.DB
}

// NewPortRegistry initialises the registry and seeds the port maps.
func NewPortRegistry(db *sql.DB) *PortRegistry {
	r := &PortRegistry{
		cdp: make(map[int]int64, portRangeMax),
		vnc: make(map[int]int64, portRangeMax),
		db:  db,
	}
	for i := 0; i < portRangeMax; i++ {
		r.cdp[CDPPortBase+i] = 0
		r.vnc[VNCPortBase+i] = 0
	}
	return r
}

// LoadFromDB marks ports as in-use for every non-terminated browser session.
// Call this once after NewPortRegistry to restore state after a server restart.
func (r *PortRegistry) LoadFromDB(ctx context.Context) error {
	rows, err := r.db.QueryContext(ctx,
		`SELECT account_id, cdp_port, vnc_port FROM browser_sessions WHERE status != 'terminated'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	r.mu.Lock()
	defer r.mu.Unlock()
	for rows.Next() {
		var acctID int64
		var cdpPort, vncPort int
		if err := rows.Scan(&acctID, &cdpPort, &vncPort); err != nil {
			continue
		}
		if cdpPort > 0 {
			r.cdp[cdpPort] = acctID
		}
		if vncPort > 0 {
			r.vnc[vncPort] = acctID
		}
	}
	return rows.Err()
}

// ReconcileFromDocker reads container labels to fill any gaps not covered by DB.
func (r *PortRegistry) ReconcileFromDocker() {
	out, err := exec.Command("docker", "ps",
		"--filter", "name="+containerPrefix,
		"--format", "{{.Names}}",
	).Output()
	if err != nil {
		return
	}

	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name = strings.TrimSpace(name)
		if !strings.HasPrefix(name, containerPrefix) {
			continue
		}

		cdpLabel, _ := exec.Command("docker", "inspect",
			"--format={{index .Config.Labels \"thg.cdp_port\"}}", name).Output()
		vncLabel, _ := exec.Command("docker", "inspect",
			"--format={{index .Config.Labels \"thg.vnc_port\"}}", name).Output()
		acctLabel, _ := exec.Command("docker", "inspect",
			"--format={{index .Config.Labels \"thg.account_id\"}}", name).Output()

		cdpPort, _ := strconv.Atoi(strings.TrimSpace(string(cdpLabel)))
		vncPort, _ := strconv.Atoi(strings.TrimSpace(string(vncLabel)))
		acctID, _ := strconv.ParseInt(strings.TrimSpace(string(acctLabel)), 10, 64)

		if acctID == 0 || (cdpPort == 0 && vncPort == 0) {
			continue
		}

		r.mu.Lock()
		if cdpPort > 0 && r.cdp[cdpPort] == 0 {
			r.cdp[cdpPort] = acctID
		}
		if vncPort > 0 && r.vnc[vncPort] == 0 {
			r.vnc[vncPort] = acctID
		}
		r.mu.Unlock()
	}
}

// ClaimPair finds a free CDP and VNC port pair for accountID and marks them allocated.
// Also writes them to the port_registry table for persistence.
func (r *PortRegistry) ClaimPair(accountID int64) (cdpPort, vncPort int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for p, owner := range r.cdp {
		if owner == accountID {
			cdpPort = p
			break
		}
	}
	for p, owner := range r.vnc {
		if owner == accountID {
			vncPort = p
			break
		}
	}
	if cdpPort > 0 && vncPort > 0 {
		return cdpPort, vncPort, nil
	}

	for p, owner := range r.cdp {
		if owner == 0 {
			cdpPort = p
			break
		}
	}
	for p, owner := range r.vnc {
		if owner == 0 {
			vncPort = p
			break
		}
	}

	if cdpPort == 0 || vncPort == 0 {
		return 0, 0, fmt.Errorf("no free ports available (max %d concurrent containers)", portRangeMax)
	}

	r.cdp[cdpPort] = accountID
	r.vnc[vncPort] = accountID

	// Persist to DB (best-effort, non-fatal)
	if r.db != nil {
		r.db.Exec(`INSERT OR REPLACE INTO port_registry (port, port_type, account_id, updated_at)
			VALUES (?, 'cdp', ?, CURRENT_TIMESTAMP)`, cdpPort, accountID)
		r.db.Exec(`INSERT OR REPLACE INTO port_registry (port, port_type, account_id, updated_at)
			VALUES (?, 'vnc', ?, CURRENT_TIMESTAMP)`, vncPort, accountID)
	}

	return cdpPort, vncPort, nil
}

// Release frees the ports previously claimed by accountID.
func (r *PortRegistry) Release(accountID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for p, owner := range r.cdp {
		if owner == accountID {
			r.cdp[p] = 0
			if r.db != nil {
				r.db.Exec(`UPDATE port_registry SET account_id=0, updated_at=CURRENT_TIMESTAMP WHERE port=?`, p)
			}
		}
	}
	for p, owner := range r.vnc {
		if owner == accountID {
			r.vnc[p] = 0
			if r.db != nil {
				r.db.Exec(`UPDATE port_registry SET account_id=0, updated_at=CURRENT_TIMESTAMP WHERE port=?`, p)
			}
		}
	}
}
