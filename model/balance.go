package model

import (
	"context"

	"github.com/mgnsk/splitgo/balance"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/pkg/snowflake"
	"github.com/mgnsk/splitgo/pkg/sqlite"
	"github.com/uptrace/bun"
)

type debtRow struct {
	GroupID  snowflake.ID `bun:"group_id"`
	Currency string       `bun:"currency"`
	FromID   snowflake.ID `bun:"from_id"`
	ToID     snowflake.ID `bun:"to_id"`
	Amount   int64        `bun:"amount"`
}

// listRawDebts aggregates non-deleted expense shares into debts: every
// participant other than the payer owes the payer their share.
func listRawDebts(ctx context.Context, db bun.IDB, groupIDs []snowflake.ID) ([]debtRow, error) {
	var rows []debtRow
	if len(groupIDs) == 0 {
		return rows, nil
	}

	if err := db.NewSelect().
		TableExpr("expense_participants AS ep").
		Join("JOIN expenses AS e ON e.id = ep.expense_id").
		ColumnExpr("e.group_id, e.currency, ep.user_id AS from_id, e.paid_by AS to_id, SUM(ep.share) AS amount").
		Where("e.group_id IN (?)", bun.List(groupIDs)).
		Where("e.deleted_at IS NULL").
		Where("ep.user_id <> e.paid_by").
		Where("ep.share > 0").
		GroupExpr("e.group_id, e.currency, ep.user_id, e.paid_by").
		Scan(ctx, &rows); err != nil {
		return nil, sqlite.NormalizeError(err)
	}

	return rows, nil
}

// GroupDebts returns the outstanding debts of a group per currency,
// simplified if the group has debt simplification enabled.
func GroupDebts(ctx context.Context, db bun.IDB, group *domain.Group) ([]domain.Debt, error) {
	rows, err := listRawDebts(ctx, db, []snowflake.ID{group.ID})
	if err != nil {
		return nil, err
	}

	return settleRows(rows, map[snowflake.ID]bool{group.ID: group.SimplifyDebts}), nil
}

// UserDebts returns all outstanding debts involving user across the given
// groups (which should be the user's groups), respecting each group's
// simplification setting.
func UserDebts(ctx context.Context, db bun.IDB, userID snowflake.ID, groups []*domain.Group) ([]domain.Debt, error) {
	ids := make([]snowflake.ID, len(groups))
	simplify := map[snowflake.ID]bool{}
	for i, g := range groups {
		ids[i] = g.ID
		simplify[g.ID] = g.SimplifyDebts
	}

	rows, err := listRawDebts(ctx, db, ids)
	if err != nil {
		return nil, err
	}

	var result []domain.Debt
	for _, d := range settleRows(rows, simplify) {
		if d.From == userID || d.To == userID {
			result = append(result, d)
		}
	}
	return result, nil
}

func settleRows(rows []debtRow, simplify map[snowflake.ID]bool) []domain.Debt {
	type key struct {
		groupID  snowflake.ID
		currency string
	}

	buckets := map[key][]balance.Debt[snowflake.ID]{}
	var keys []key
	for _, r := range rows {
		k := key{r.GroupID, r.Currency}
		if _, ok := buckets[k]; !ok {
			keys = append(keys, k)
		}
		buckets[k] = append(buckets[k], balance.Debt[snowflake.ID]{From: r.FromID, To: r.ToID, Amount: r.Amount})
	}

	var result []domain.Debt
	for _, k := range keys {
		for _, d := range balance.Settle(buckets[k], simplify[k.groupID]) {
			result = append(result, domain.Debt{Debt: d, Currency: k.currency, GroupID: k.groupID})
		}
	}

	return result
}
