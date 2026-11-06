package server

import (
	"context"

	"github.com/alexedwards/scs/v2"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/webkit/session"
	"github.com/mgnsk/webkit/snowflake"
	"github.com/mgnsk/webkit/web"
	"github.com/uptrace/bun"
)

// MiddlewareFunc is a HTTP middleware function.
type MiddlewareFunc = web.Middleware

// WithMiddleware applies middleware to run in order before handler.
var WithMiddleware = web.Chain

// NewSessionMiddleware creates a new session middleware.
func NewSessionMiddleware(sm *scs.SessionManager) MiddlewareFunc {
	return session.Middleware(sm)
}

// NewUserMiddleware creates a middleware that loads the logged in user
// into the request context. Sessions of deleted users are destroyed.
func NewUserMiddleware(db *bun.DB, sm *scs.SessionManager) MiddlewareFunc {
	return session.LoadUser(sm, userKey, func(ctx context.Context, sm *scs.SessionManager) (*domain.User, error) {
		id := sm.GetInt64(ctx, SessionKeyUserID)
		if id == 0 {
			return nil, nil
		}
		return model.GetUser(ctx, db, snowflake.ID(id))
	}, "/login")
}

// RequireUser is a middleware that requires a logged in user.
// Anonymous page requests are redirected to the login page.
var RequireUser = session.RequireUser(userKey, "/login")
