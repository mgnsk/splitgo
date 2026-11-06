package server

import (
	"context"

	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/webkit/web"
)

var userKey web.ContextKey[domain.User]

// GetUser returns the current user from context if present.
func GetUser(ctx context.Context) *domain.User {
	return userKey.Get(ctx)
}

// WithUser returns a context with user.
func WithUser(ctx context.Context, user *domain.User) context.Context {
	return userKey.With(ctx, user)
}
