package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// profileLockFile is the lock-file name placed inside each account's
// profile directory. Whoever holds an exclusive O_CREATE|O_EXCL handle
// on this file owns the right to launch a Chrome container against the
// underlying --user-data-dir.
//
// This is intentionally cross-process: an in-memory mutex on the Manager
// only protects goroutines inside the same API process. The same host
// can run two API processes during a blue/green deploy, or have a stale
// chrome process holding the user_data_dir SingletonLock from a prior
// crash. The lock-file plus PID stamp lets the second process detect
// the conflict before it tries to docker-run a second container against
// the same volume.
const profileLockFile = ".thg-profile.lock"

// profileLockTTL is how long a stamped PID file is considered live before
// we treat it as stale (e.g. a crashed API that left the file behind).
// 30 minutes is generous enough for a slow Chrome startup but short
// enough that operators don't sit waiting after a crash.
const profileLockTTL = 30 * time.Minute

// ProfileLock represents an exclusive claim on an account's profile dir.
// Release MUST be called when the holder is done with it.
type ProfileLock struct {
	path string
}

// AcquireProfileLock takes an exclusive lock on the given profile dir.
// Behaviour:
//
//   - If the lock-file does not exist: create it with this PID + timestamp,
//     return success.
//   - If the lock-file exists and is fresh (< profileLockTTL): return an
//     error so the caller refuses to mount the same dir twice.
//   - If the lock-file exists but is older than profileLockTTL: assume the
//     previous owner crashed, overwrite the stamp and proceed.
//
// Note: this guards bind-mount + Chrome launch races. Chrome itself also
// writes a Singleton* set of files inside the profile dir; both layers
// together ensure no two Chromes share a profile.
func AcquireProfileLock(profileDir string) (*ProfileLock, error) {
	if profileDir == "" {
		return nil, errors.New("profile dir is empty")
	}
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return nil, fmt.Errorf("ensure profile dir: %w", err)
	}
	path := filepath.Join(profileDir, profileLockFile)

	stamp := fmt.Sprintf("%d\n%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))

	// Try exclusive create first. If it succeeds nobody else holds the dir.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err == nil {
		_, _ = f.WriteString(stamp)
		_ = f.Close()
		return &ProfileLock{path: path}, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("create profile lock: %w", err)
	}

	// Lock file exists. Decide whether it is stale.
	fresh, owner, err := profileLockFresh(path)
	if err != nil {
		// We cannot read it — fail closed; prefer leaving the live profile
		// alone over racing with a real owner.
		return nil, fmt.Errorf("inspect profile lock: %w", err)
	}
	if fresh {
		return nil, fmt.Errorf("profile dir is in use by %s", owner)
	}

	// Stale: overwrite atomically.
	if err := os.WriteFile(path, []byte(stamp), 0644); err != nil {
		return nil, fmt.Errorf("refresh profile lock: %w", err)
	}
	return &ProfileLock{path: path}, nil
}

// Release deletes the lock file. Called on container Stop or after a
// failed Start so the next caller can take over.
func (l *ProfileLock) Release() {
	if l == nil || l.path == "" {
		return
	}
	_ = os.Remove(l.path)
	l.path = ""
}

// profileLockFresh reports whether the lock file at path was written
// less than profileLockTTL ago. Returns the owner string ("pid <pid> @
// <timestamp>") for diagnostics.
func profileLockFresh(path string) (bool, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, "", err
	}
	parts := strings.Split(strings.TrimSpace(string(data)), "\n")
	pid := "?"
	stamp := ""
	if len(parts) >= 1 {
		pid = strings.TrimSpace(parts[0])
	}
	if len(parts) >= 2 {
		stamp = strings.TrimSpace(parts[1])
	}
	owner := fmt.Sprintf("pid %s", pid)
	if stamp != "" {
		owner = fmt.Sprintf("pid %s @ %s", pid, stamp)
	}

	if stamp == "" {
		// No timestamp — fall back to mtime.
		fi, err := os.Stat(path)
		if err != nil {
			return false, owner, err
		}
		return time.Since(fi.ModTime()) < profileLockTTL, owner, nil
	}
	t, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		// Malformed timestamp; rely on mtime.
		fi, err2 := os.Stat(path)
		if err2 != nil {
			return false, owner, err2
		}
		return time.Since(fi.ModTime()) < profileLockTTL, owner, nil
	}
	return time.Since(t) < profileLockTTL, owner, nil
}

