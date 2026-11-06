package model

import (
	"context"
	"crypto/subtle"
	"time"

	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/webkit"
	"github.com/mgnsk/webkit/sqlite"
	"github.com/uptrace/bun"
)

// LoginToken is a magic link login token with an optional one-time code.
// Only hashes of the token and code are stored.
type LoginToken struct {
	TokenHash string `bun:"token_hash,pk"`
	CodeHash  string `bun:"code_hash"`
	Attempts  int    `bun:"attempts"`
	Email     string `bun:"email"`
	CreatedAt int64  `bun:"created_at"`
	ExpiresAt int64  `bun:"expires_at"`

	bun.BaseModel `bun:"login_tokens"`
}

// LoginFailure is a failed sign-in code attempt.
type LoginFailure struct {
	Email     string `bun:"email"`
	CreatedAt int64  `bun:"created_at"`

	bun.BaseModel `bun:"login_failures"`
}

// MaxCodeAttempts is the number of wrong codes after which a token's code
// stops working (the link still works).
const MaxCodeAttempts = 5

// InsertLoginToken stores a login token hash for email. codeHash may be
// empty when the token has no code.
func InsertLoginToken(ctx context.Context, db bun.IDB, tokenHash, codeHash, email string, now time.Time, ttl time.Duration) error {
	return sqlite.WithErrorChecking(db.NewInsert().Model(&LoginToken{
		TokenHash: tokenHash,
		CodeHash:  codeHash,
		Email:     domain.NormalizeEmail(email),
		CreatedAt: now.Unix(),
		ExpiresAt: now.Add(ttl).Unix(),
	}).Exec(ctx))
}

// ConsumeLoginCode verifies a sign-in code for email against the newest valid
// token with a code. hashCode computes the code hash for a token hash. On
// success the token is deleted. A wrong code counts an attempt on the token
// and records a failure for email. It returns NotFound if there is no usable
// token and InvalidValue for a wrong code.
func ConsumeLoginCode(
	ctx context.Context,
	db bun.IDB,
	email string,
	hashCode func(tokenHash string) string,
	now time.Time,
) error {
	email = domain.NormalizeEmail(email)
	wrong := false

	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		m := &LoginToken{}
		if err := tx.NewSelect().Model(m).
			Where("email = ?", email).
			Where("code_hash <> ''").
			Where("attempts < ?", MaxCodeAttempts).
			Where("expires_at > ?", now.Unix()).
			Order("created_at DESC").
			Limit(1).
			Scan(ctx); err != nil {
			return sqlite.NormalizeError(err)
		}

		if subtle.ConstantTimeCompare([]byte(hashCode(m.TokenHash)), []byte(m.CodeHash)) == 1 {
			return sqlite.WithErrorChecking(tx.NewDelete().Model(m).WherePK().Exec(ctx))
		}

		if _, err := tx.NewUpdate().Model(m).
			Set("attempts = attempts + 1").
			WherePK().
			Exec(ctx); err != nil {
			return sqlite.NormalizeError(err)
		}

		if _, err := tx.NewInsert().Model(&LoginFailure{Email: email, CreatedAt: now.Unix()}).Exec(ctx); err != nil {
			return sqlite.NormalizeError(err)
		}

		// Commit the attempt, then report the wrong code.
		wrong = true
		return nil
	})
	if err != nil {
		return err
	}

	if wrong {
		return webkit.InvalidValue.New("Wrong code")
	}

	return nil
}

// CountLoginFailures counts failed code attempts for email since a time.
func CountLoginFailures(ctx context.Context, db bun.IDB, email string, since time.Time) (int, error) {
	n, err := db.NewSelect().Model((*LoginFailure)(nil)).
		Where("email = ?", domain.NormalizeEmail(email)).
		Where("created_at > ?", since.Unix()).
		Count(ctx)
	return int(n), sqlite.NormalizeError(err)
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

// LoginFailureWindow is how long failed code attempts are remembered.
const LoginFailureWindow = 24 * time.Hour

// DeleteExpiredLoginTokens deletes expired login tokens and old failures.
func DeleteExpiredLoginTokens(ctx context.Context, db bun.IDB, now time.Time) error {
	if _, err := db.NewDelete().Model((*LoginToken)(nil)).
		Where("expires_at <= ?", now.Unix()).
		Exec(ctx); err != nil {
		return sqlite.NormalizeError(err)
	}

	_, err := db.NewDelete().Model((*LoginFailure)(nil)).
		Where("created_at <= ?", now.Add(-LoginFailureWindow).Unix()).
		Exec(ctx)
	return sqlite.NormalizeError(err)
}
