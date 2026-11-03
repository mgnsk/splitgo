package server

import (
	"context"

	"github.com/mgnsk/splitgo/domain"
)

type userCtxKey struct{}

// GetUser returns the current user from context if present.
func GetUser(ctx context.Context) *domain.User {
	if user, ok := ctx.Value(userCtxKey{}).(*domain.User); ok {
		return user
	}
	return nil
}

// WithUser returns a context with user.
func WithUser(ctx context.Context, user *domain.User) context.Context {
	return context.WithValue(ctx, userCtxKey{}, user)
}
