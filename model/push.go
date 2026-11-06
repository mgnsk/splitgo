package model

import (
	"context"
	"errors"
	"time"

	"github.com/mgnsk/splitgo"
	"github.com/mgnsk/webkit/snowflake"
	"github.com/mgnsk/webkit/sqlite"
	"github.com/samber/lo"
	"github.com/uptrace/bun"
)

// PushSubscription is a Web Push subscription of a user's device.
type PushSubscription struct {
	Endpoint  string       `bun:"endpoint,pk"`
	UserID    snowflake.ID `bun:"user_id"`
	P256dh    string       `bun:"p256dh"`
	Auth      string       `bun:"auth"`
	CreatedAt int64        `bun:"created_at"`

	bun.BaseModel `bun:"push_subscriptions"`
}

// VAPIDKeys is the server's VAPID key pair.
type VAPIDKeys struct {
	ID         int    `bun:"id,pk"`
	PublicKey  string `bun:"public_key"`
	PrivateKey string `bun:"private_key"`

	bun.BaseModel `bun:"vapid_keys"`
}

// SavePushSubscription stores a subscription. A device that subscribes again,
// possibly signed in as another user, replaces its previous subscription.
func SavePushSubscription(ctx context.Context, db bun.IDB, s *PushSubscription, now time.Time) error {
	s.CreatedAt = now.UnixMilli()
	_, err := db.NewInsert().Model(s).
		On("CONFLICT (endpoint) DO UPDATE").
		Set("user_id = EXCLUDED.user_id").
		Set("p256dh = EXCLUDED.p256dh").
		Set("auth = EXCLUDED.auth").
		Set("created_at = EXCLUDED.created_at").
		Exec(ctx)
	return sqlite.NormalizeError(err)
}

// DeleteUserPushSubscription deletes a user's subscription by endpoint.
func DeleteUserPushSubscription(ctx context.Context, db bun.IDB, userID snowflake.ID, endpoint string) error {
	_, err := db.NewDelete().Model((*PushSubscription)(nil)).
		Where("endpoint = ?", endpoint).
		Where("user_id = ?", userID).
		Exec(ctx)
	return sqlite.NormalizeError(err)
}

// DeletePushSubscription deletes a subscription the push service no longer
// accepts.
func DeletePushSubscription(ctx context.Context, db bun.IDB, endpoint string) error {
	_, err := db.NewDelete().Model((*PushSubscription)(nil)).Where("endpoint = ?", endpoint).Exec(ctx)
	return sqlite.NormalizeError(err)
}

// ListPushSubscriptions lists the subscriptions of users.
func ListPushSubscriptions(ctx context.Context, db bun.IDB, userIDs []snowflake.ID) ([]*PushSubscription, error) {
	var subs []*PushSubscription
	if len(userIDs) == 0 {
		return subs, nil
	}
	if err := db.NewSelect().Model(&subs).
		Where("user_id IN (?)", bun.List(lo.Uniq(userIDs))).
		Order("created_at ASC").
		Scan(ctx); err != nil {
		return nil, sqlite.NormalizeError(err)
	}
	return subs, nil
}

// GetOrCreateVAPIDKeys returns the stored VAPID keys, storing the keys
// returned by generate on first use.
func GetOrCreateVAPIDKeys(ctx context.Context, db bun.IDB, generate func() (private, public string, err error)) (*VAPIDKeys, error) {
	keys := &VAPIDKeys{}
	err := db.NewSelect().Model(keys).Where("id = 1").Scan(ctx)
	if err == nil {
		return keys, nil
	}
	if err = sqlite.NormalizeError(err); !errors.Is(err, splitgo.NotFound) {
		return nil, err
	}

	private, public, err := generate()
	if err != nil {
		return nil, err
	}

	keys = &VAPIDKeys{ID: 1, PublicKey: public, PrivateKey: private}
	if _, err := db.NewInsert().Model(keys).On("CONFLICT DO NOTHING").Exec(ctx); err != nil {
		return nil, sqlite.NormalizeError(err)
	}

	// Another process may have won the race; read back what is stored.
	if err := db.NewSelect().Model(keys).Where("id = 1").Scan(ctx); err != nil {
		return nil, sqlite.NormalizeError(err)
	}
	return keys, nil
}
