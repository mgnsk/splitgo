package model

import (
	"context"
	"time"

	"github.com/mgnsk/webkit/snowflake"
	"github.com/mgnsk/webkit/sqlite"
	"github.com/uptrace/bun"
)

// Invite limits: a user may send MaxInvites invite emails per InviteWindow.
const (
	MaxInvites   = 20
	InviteWindow = 24 * time.Hour
)

// Invite is an invite email sent by a user.
type Invite struct {
	UserID    snowflake.ID `bun:"user_id"`
	CreatedAt int64        `bun:"created_at"`

	bun.BaseModel `bun:"invites"`
}

// InsertInvite records an invite email sent by user.
func InsertInvite(ctx context.Context, db bun.IDB, userID snowflake.ID, now time.Time) error {
	_, err := db.NewInsert().Model(&Invite{UserID: userID, CreatedAt: now.Unix()}).Exec(ctx)
	return sqlite.NormalizeError(err)
}

// CanInvite reports whether user may send another invite email.
func CanInvite(ctx context.Context, db bun.IDB, userID snowflake.ID, now time.Time) (bool, error) {
	n, err := db.NewSelect().Model((*Invite)(nil)).
		Where("user_id = ?", userID).
		Where("created_at > ?", now.Add(-InviteWindow).Unix()).
		Count(ctx)
	return n < MaxInvites, sqlite.NormalizeError(err)
}

// DeleteOldInvites deletes invites older than the rate limit window.
func DeleteOldInvites(ctx context.Context, db bun.IDB, now time.Time) error {
	_, err := db.NewDelete().Model((*Invite)(nil)).
		Where("created_at <= ?", now.Add(-InviteWindow).Unix()).
		Exec(ctx)
	return sqlite.NormalizeError(err)
}
