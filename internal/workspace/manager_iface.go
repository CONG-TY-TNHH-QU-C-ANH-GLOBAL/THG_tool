package workspace

// ManagerIface is the interface implemented by *Manager.
// Used by HealthChecker and RestartController to avoid copying the mutex-bearing struct.
type ManagerIface interface {
	Start(accountID int64, accountName string) (*Instance, error)
	Stop(accountID int64)
	Get(accountID int64) *Instance
	List() []*Instance
	ReconcileRunning()
	StopAll()
}
