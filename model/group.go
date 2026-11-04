package model

import (
	"context"

	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/pkg/snowflake"
	"github.com/mgnsk/splitgo/pkg/sqlite"
	"github.com/samber/lo"
	"github.com/uptrace/bun"
)

// Group is the group database model.
type Group struct {
	ID            snowflake.ID `bun:"id,pk"`
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
	if err := db.NewSelect().Model(m).Where("invite_code = ?", code).Scan(ctx); err != nil {
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

// SharesGroup reports whether two users are members of a common group.
func SharesGroup(ctx context.Context, db bun.IDB, a, b snowflake.ID) (bool, error) {
	ok, err := db.NewSelect().Model((*GroupMember)(nil)).
		Where("user_id = ?", a).
		Where("group_id IN (SELECT group_id FROM group_members WHERE user_id = ?)", b).
		Exists(ctx)
	return ok, sqlite.NormalizeError(err)
}

func groupFromDomain(g *domain.Group) *Group {
	return &Group{
		ID:            g.ID,
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
		Name:          m.Name,
		Currency:      m.Currency,
		SimplifyDebts: m.SimplifyDebts,
		InviteCode:    m.InviteCode,
		CreatedBy:     m.CreatedBy,
	}
}
