// Package model implements database access.
package model

import (
	"context"

	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/pkg/snowflake"
	"github.com/mgnsk/splitgo/pkg/sqlite"
	"github.com/samber/lo"
	"github.com/uptrace/bun"
)

// User is the user database model.
type User struct {
	ID       snowflake.ID `bun:"id,pk"`
	Email    string       `bun:"email"`
	Name     string       `bun:"name"`
	Currency string       `bun:"currency"`
	Theme    string       `bun:"theme"`

	bun.BaseModel `bun:"users"`
}

// InsertUser inserts a user.
func InsertUser(ctx context.Context, db bun.IDB, user *domain.User) error {
	return sqlite.WithErrorChecking(db.NewInsert().Model(userFromDomain(user)).Exec(ctx))
}

// UpdateUser updates a user's profile.
func UpdateUser(ctx context.Context, db bun.IDB, user *domain.User) error {
	return sqlite.WithErrorChecking(db.NewUpdate().Model(userFromDomain(user)).
		Column("name", "currency", "theme").
		WherePK().
		Exec(ctx))
}

// GetUser returns a user by ID.
func GetUser(ctx context.Context, db bun.IDB, id snowflake.ID) (*domain.User, error) {
	m := &User{}
	if err := db.NewSelect().Model(m).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, sqlite.NormalizeError(err)
	}
	return userToDomain(m), nil
}

// GetUserByEmail returns a user by email.
func GetUserByEmail(ctx context.Context, db bun.IDB, email string) (*domain.User, error) {
	m := &User{}
	if err := db.NewSelect().Model(m).Where("email = ?", domain.NormalizeEmail(email)).Scan(ctx); err != nil {
		return nil, sqlite.NormalizeError(err)
	}
	return userToDomain(m), nil
}

// ListUsersByIDs returns users with the given IDs keyed by ID.
func ListUsersByIDs(ctx context.Context, db bun.IDB, ids []snowflake.ID) (map[snowflake.ID]*domain.User, error) {
	result := map[snowflake.ID]*domain.User{}
	if len(ids) == 0 {
		return result, nil
	}

	var models []*User
	if err := db.NewSelect().Model(&models).Where("id IN (?)", bun.List(lo.Uniq(ids))).Scan(ctx); err != nil {
		return nil, sqlite.NormalizeError(err)
	}

	for _, m := range models {
		result[m.ID] = userToDomain(m)
	}
	return result, nil
}

// CountUsers returns the number of users.
func CountUsers(ctx context.Context, db bun.IDB) (int64, error) {
	n, err := db.NewSelect().Model((*User)(nil)).Count(ctx)
	return n, sqlite.NormalizeError(err)
}

func userFromDomain(u *domain.User) *User {
	return &User{
		ID:       u.ID,
		Email:    domain.NormalizeEmail(u.Email),
		Name:     u.Name,
		Currency: u.Currency,
		Theme:    themeOrDefault(u.Theme),
	}
}

func userToDomain(m *User) *domain.User {
	return &domain.User{
		ID:       m.ID,
		Email:    m.Email,
		Name:     m.Name,
		Currency: m.Currency,
		Theme:    domain.Theme(themeOrDefault(domain.Theme(m.Theme))),
	}
}

func themeOrDefault(t domain.Theme) string {
	if !t.Valid() {
		return string(domain.ThemeSystem)
	}
	return string(t)
}
