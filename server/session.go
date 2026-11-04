package server

import (
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
)

// Session keys.
const (
	SessionKeyUserID       = "user_id"
	SessionKeyFlashSuccess = "flash-success"
)

// NewSessionManager creates a new session manager.
func NewSessionManager(store scs.Store, secure bool) *scs.SessionManager {
	sm := scs.New()
	sm.Store = store
	sm.HashTokenInStore = true
	// Long sessions so the installed app (PWA) stays signed in. No idle
	// timeout: the session lasts a year from sign-in.
	sm.Lifetime = 365 * 24 * time.Hour
	sm.Cookie.Name = "session_id"
	sm.Cookie.Domain = ""
	sm.Cookie.HttpOnly = true
	sm.Cookie.Path = "/"
	sm.Cookie.Persist = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Secure = secure

	sm.ErrorFunc = func(_ http.ResponseWriter, _ *http.Request, err error) {
		panic(err)
	}

	return sm
}
