package model

import (
	"context"
	"time"

	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/pkg/sqlite"
	"github.com/uptrace/bun"
)

// LoginToken is a magic link login token. Only the token hash is stored.
type LoginToken struct {
	TokenHash string `bun:"token_hash,pk"`
	Email     string `bun:"email"`
	CreatedAt int64  `bun:"created_at"`
	ExpiresAt int64  `bun:"expires_at"`

	bun.BaseModel `bun:"login_tokens"`
}

// InsertLoginToken stores a login token hash for email.
func InsertLoginToken(ctx context.Context, db bun.IDB, tokenHash, email string, now time.Time, ttl time.Duration) error {
	return sqlite.WithErrorChecking(db.NewInsert().Model(&LoginToken{
		TokenHash: tokenHash,
		Email:     domain.NormalizeEmail(email),
		CreatedAt: now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
	}).Exec(ctx))
}

// GetLoginToken returns a valid (unexpired) login token's email.
func GetLoginToken(ctx context.Context, db bun.IDB, tokenHash string, now time.Time) (string, error) {
	m := &LoginToken{}
	if err := db.NewSelect().Model(m).
		Where("token_hash = ?", tokenHash).
		Where("expires_at > ?", now.Unix()).
		Scan(ctx); err != nil {
		return "", sqlite.NormalizeError(err)
	}
	return m.Email, nil
}

// ConsumeLoginToken deletes a valid login token and returns its email.
// It returns NotFound if the token does not exist or has expired.
func ConsumeLoginToken(ctx context.Context, db bun.IDB, tokenHash string, now time.Time) (string, error) {
	m := &LoginToken{}
	if err := db.NewDelete().Model(m).
		Where("token_hash = ?", tokenHash).
		Where("expires_at > ?", now.Unix()).
		Returning("email").
		Scan(ctx); err != nil {
		return "", sqlite.NormalizeError(err)
	}
	return m.Email, nil
}

// LastLoginTokenTime returns when the last login token for email was created.
// It returns the zero time if none exists.
func LastLoginTokenTime(ctx context.Context, db bun.IDB, email string) (time.Time, error) {
	var createdAt int64
	if err := db.NewSelect().Model((*LoginToken)(nil)).
		ColumnExpr("COALESCE(MAX(created_at), 0)").
		Where("email = ?", domain.NormalizeEmail(email)).
		Scan(ctx, &createdAt); err != nil {
		return time.Time{}, sqlite.NormalizeError(err)
	}
	if createdAt == 0 {
		return time.Time{}, nil
	}
	return time.Unix(createdAt, 0), nil
}

// DeleteExpiredLoginTokens deletes expired login tokens.
func DeleteExpiredLoginTokens(ctx context.Context, db bun.IDB, now time.Time) error {
	_, err := db.NewDelete().Model((*LoginToken)(nil)).
		Where("expires_at <= ?", now.Unix()).
		Exec(ctx)
	return sqlite.NormalizeError(err)
}
