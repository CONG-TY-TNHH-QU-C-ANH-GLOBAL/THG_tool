// Domain: sessions (see internal/store/DOMAINS.md)
package store

import (
	"github.com/thg/scraper/internal/store/sessions"
)

// BrowserSession is an alias of [sessions.BrowserSession] for source
// compatibility. New code should import "internal/store/sessions" and use
// [sessions.BrowserSession] directly.
type BrowserSession = sessions.BrowserSession
