package reel

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// downloadTo GETs url and streams the body to destPath, creating parent dirs as needed.
// A non-2xx response is an error (the partial file is removed) so a provider 404/expired
// URL degrades honestly instead of writing a truncated clip.
func downloadTo(ctx context.Context, client *http.Client, url, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download %s: status %d", url, resp.StatusCode)
	}
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(destPath)
		return err
	}
	return f.Close()
}

// waitCtx sleeps for d but aborts early if ctx is cancelled (poll loops must not outlive the
// render deadline). Returns ctx.Err() on cancellation.
func waitCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
