package model

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"

	"github.com/mgnsk/splitgo"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/webkit/snowflake"
	"github.com/mgnsk/webkit/sqlite"
	"github.com/samber/lo"
	"github.com/uptrace/bun"
)

// Group is the group database model.
type Group struct {
	ID            snowflake.ID `bun:"id,pk"`
	Kind          string       `bun:"kind"`
	DirectKey     *string      `bun:"direct_key"`
	Name          string       `bun:"name"`
	Currency      string       `bun:"currency"`
	SimplifyDebts bool         `bun:"simplify_debts"`
	InviteCode    string       `bun:"invite_code"`
	CreatedBy     snowflake.ID `bun:"created_by"`

	bun.BaseModel `bun:"groups,alias:g"`
}

// GroupMember is the group membership database model.
type GroupMember struct {
	GroupID snowflake.ID `bun:"group_id,pk"`
	UserID  snowflake.ID `bun:"user_id,pk"`

	bun.BaseModel `bun:"group_members"`
}

// InsertGroup inserts a group and adds its creator as a member.
func InsertGroup(ctx context.Context, db bun.IDB, group *domain.Group) error {
	if group.Kind == "" {
		group.Kind = domain.GroupKindGroup
	}
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := sqlite.WithErrorChecking(tx.NewInsert().Model(groupFromDomain(group)).Exec(ctx)); err != nil {
			return err
		}
		return AddGroupMember(ctx, tx, group.ID, group.CreatedBy)
	})
}

// UpdateGroup updates a group's settings.
func UpdateGroup(ctx context.Context, db bun.IDB, group *domain.Group) error {
	return sqlite.WithErrorChecking(db.NewUpdate().Model(groupFromDomain(group)).
		Column("name", "currency", "simplify_debts").
		WherePK().
		Exec(ctx))
}

// SetGroupInviteCode replaces a group's invite code, so the old invite link
// stops working.
func SetGroupInviteCode(ctx context.Context, db bun.IDB, groupID snowflake.ID, code string) error {
	return sqlite.WithErrorChecking(db.NewUpdate().Model((*Group)(nil)).
		Set("invite_code = ?", code).
		Where("id = ?", groupID).
		Exec(ctx))
}

// DeleteGroup deletes a group with its memberships and expenses.
func DeleteGroup(ctx context.Context, db bun.IDB, id snowflake.ID) error {
	return sqlite.WithErrorChecking(db.NewDelete().Model((*Group)(nil)).Where("id = ?", id).Exec(ctx))
}

// GetGroup returns a group.
func GetGroup(ctx context.Context, db bun.IDB, id snowflake.ID) (*domain.Group, error) {
	m := &Group{}
	if err := db.NewSelect().Model(m).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, sqlite.NormalizeError(err)
	}
	return groupToDomain(m), nil
}

// GetGroupByInviteCode returns a group by its invite code.
func GetGroupByInviteCode(ctx context.Context, db bun.IDB, code string) (*domain.Group, error) {
	m := &Group{}
	if err := db.NewSelect().Model(m).
		Where("invite_code = ?", code).
		Where("kind = ?", domain.GroupKindGroup).
		Scan(ctx); err != nil {
		return nil, sqlite.NormalizeError(err)
	}
	return groupToDomain(m), nil
}

// GetUserGroup returns a group if user is a member, otherwise NotFound.
func GetUserGroup(ctx context.Context, db bun.IDB, userID, groupID snowflake.ID) (*domain.Group, error) {
	m := &Group{}
	if err := db.NewSelect().Model(m).
		Where("g.id = ?", groupID).
		Where("EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = g.id AND gm.user_id = ?)", userID).
		Scan(ctx); err != nil {
		return nil, sqlite.NormalizeError(err)
	}
	return groupToDomain(m), nil
}

// ListUserGroups lists groups the user is a member of, by name.
func ListUserGroups(ctx context.Context, db bun.IDB, userID snowflake.ID) ([]*domain.Group, error) {
	var models []*Group
	if err := db.NewSelect().Model(&models).
		Join("JOIN group_members AS gm ON gm.group_id = g.id").
		Where("gm.user_id = ?", userID).
		OrderExpr("g.name COLLATE NOCASE ASC, g.id ASC").
		Scan(ctx); err != nil {
		return nil, sqlite.NormalizeError(err)
	}
	return lo.Map(models, func(m *Group, _ int) *domain.Group { return groupToDomain(m) }), nil
}

// AddGroupMember adds a user to a group. Adding an existing member is a no-op.
func AddGroupMember(ctx context.Context, db bun.IDB, groupID, userID snowflake.ID) error {
	_, err := db.NewInsert().Model(&GroupMember{GroupID: groupID, UserID: userID}).
		On("CONFLICT DO NOTHING").
		Exec(ctx)
	return sqlite.NormalizeError(err)
}

// RemoveGroupMember removes a user from a group.
func RemoveGroupMember(ctx context.Context, db bun.IDB, groupID, userID snowflake.ID) error {
	return sqlite.WithErrorChecking(db.NewDelete().Model((*GroupMember)(nil)).
		Where("group_id = ?", groupID).
		Where("user_id = ?", userID).
		Exec(ctx))
}

// ListGroupMembers lists group members ordered by ID.
func ListGroupMembers(ctx context.Context, db bun.IDB, groupID snowflake.ID) ([]*domain.User, error) {
	var models []*User
	if err := db.NewSelect().Model(&models).
		Where("id IN (SELECT user_id FROM group_members WHERE group_id = ?)", groupID).
		Order("id ASC").
		Scan(ctx); err != nil {
		return nil, sqlite.NormalizeError(err)
	}
	return lo.Map(models, func(m *User, _ int) *domain.User { return userToDomain(m) }), nil
}

// IsGroupMember reports whether user is a member of group.
func IsGroupMember(ctx context.Context, db bun.IDB, groupID, userID snowflake.ID) (bool, error) {
	ok, err := db.NewSelect().Model((*GroupMember)(nil)).
		Where("group_id = ?", groupID).
		Where("user_id = ?", userID).
		Exists(ctx)
	return ok, sqlite.NormalizeError(err)
}

// Friend is a friend of a user with their shared direct group.
type Friend struct {
	User          *domain.User
	DirectGroupID snowflake.ID
}

func directKey(a, b snowflake.ID) string {
	if a > b {
		a, b = b, a
	}
	return a.String() + ":" + b.String()
}

// GetDirectGroup returns the direct group of two users.
func GetDirectGroup(ctx context.Context, db bun.IDB, a, b snowflake.ID) (*domain.Group, error) {
	m := &Group{}
	if err := db.NewSelect().Model(m).Where("direct_key = ?", directKey(a, b)).Scan(ctx); err != nil {
		return nil, sqlite.NormalizeError(err)
	}
	return groupToDomain(m), nil
}

// GetOrCreateDirectGroup returns the direct group of two users, creating it
// with both as members. created reports whether it was created.
func GetOrCreateDirectGroup(ctx context.Context, db bun.IDB, a, b snowflake.ID, currency string) (group *domain.Group, created bool, err error) {
	if a == b {
		return nil, false, splitgo.InvalidValue.New("Can not be friends with yourself")
	}

	err = db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		group, err = GetDirectGroup(ctx, tx, a, b)
		if err == nil || !errors.Is(err, splitgo.NotFound) {
			return err
		}

		key := directKey(a, b)
		m := &Group{
			ID:         snowflake.Generate(),
			Kind:       string(domain.GroupKindDirect),
			DirectKey:  &key,
			Currency:   currency,
			InviteCode: randomCode(),
			CreatedBy:  a,
		}
		if err := sqlite.WithErrorChecking(tx.NewInsert().Model(m).Exec(ctx)); err != nil {
			return err
		}
		for _, id := range []snowflake.ID{a, b} {
			if err := AddGroupMember(ctx, tx, m.ID, id); err != nil {
				return err
			}
		}

		group, created = groupToDomain(m), true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return group, created, nil
}

// ListFriends lists the user's friends by name.
func ListFriends(ctx context.Context, db bun.IDB, userID snowflake.ID) ([]Friend, error) {
	var rows []struct {
		User
		GroupID snowflake.ID `bun:"group_id"`
	}
	if err := db.NewSelect().
		TableExpr("groups AS g").
		Join("JOIN group_members AS mine ON mine.group_id = g.id AND mine.user_id = ?", userID).
		Join("JOIN group_members AS theirs ON theirs.group_id = g.id AND theirs.user_id <> ?", userID).
		Join("JOIN users AS u ON u.id = theirs.user_id").
		ColumnExpr("u.*, g.id AS group_id").
		Where("g.kind = ?", domain.GroupKindDirect).
		OrderExpr("CASE WHEN u.name = '' THEN u.email ELSE u.name END COLLATE NOCASE ASC, u.id ASC").
		Scan(ctx, &rows); err != nil {
		return nil, sqlite.NormalizeError(err)
	}

	friends := make([]Friend, len(rows))
	for i, r := range rows {
		friends[i] = Friend{User: userToDomain(&r.User), DirectGroupID: r.GroupID}
	}
	return friends, nil
}

func randomCode() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// SharesGroup reports whether two users are members of a common group.
func SharesGroup(ctx context.Context, db bun.IDB, a, b snowflake.ID) (bool, error) {
	ok, err := db.NewSelect().Model((*GroupMember)(nil)).
		Where("user_id = ?", a).
		Where("group_id IN (SELECT group_id FROM group_members WHERE user_id = ?)", b).
		Exists(ctx)
	return ok, sqlite.NormalizeError(err)
}

func groupFromDomain(g *domain.Group) *Group {
	kind := g.Kind
	if kind == "" {
		kind = domain.GroupKindGroup
	}
	return &Group{
		ID:            g.ID,
		Kind:          string(kind),
		Name:          g.Name,
		Currency:      g.Currency,
		SimplifyDebts: g.SimplifyDebts,
		InviteCode:    g.InviteCode,
		CreatedBy:     g.CreatedBy,
	}
}

func groupToDomain(m *Group) *domain.Group {
	return &domain.Group{
		ID:            m.ID,
		Kind:          domain.GroupKind(m.Kind),
		Name:          m.Name,
		Currency:      m.Currency,
		SimplifyDebts: m.SimplifyDebts,
		InviteCode:    m.InviteCode,
		CreatedBy:     m.CreatedBy,
	}
}
