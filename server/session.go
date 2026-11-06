package server

import (
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/mgnsk/webkit/session"
)

// Session keys.
const (
	SessionKeyUserID = "user_id"
)

// NewSessionManager creates a new session manager.
func NewSessionManager(store scs.Store, secure bool) *scs.SessionManager {
	return session.New(store, session.Options{
		Secure: secure,
		// Long sessions so the installed app (PWA) stays signed in. No idle
		// timeout: the session lasts a year from sign-in.
		Lifetime: 365 * 24 * time.Hour,
	})
}
