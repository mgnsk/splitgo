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

// NewUserMiddleware creates a middleware that loads the logged in user
// into the request context. Sessions of deleted users are destroyed.
func NewUserMiddleware(db *bun.DB, sm *scs.SessionManager) web.Middleware {
	return session.LoadUser(sm, UserKey, func(ctx context.Context, sm *scs.SessionManager) (*domain.User, error) {
		id := sm.GetInt64(ctx, SessionKeyUserID)
		if id == 0 {
			return nil, nil
		}
		return model.GetUser(ctx, db, snowflake.ID(id))
	}, "/login")
}
