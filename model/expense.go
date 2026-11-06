package model

import (
	"context"
	"time"

	"github.com/mgnsk/splitgo"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/split"
	"github.com/mgnsk/webkit/snowflake"
	"github.com/mgnsk/webkit/sqlite"
	"github.com/samber/lo"
	"github.com/uptrace/bun"
)

// Expense is the expense database model.
type Expense struct {
	ID          snowflake.ID `bun:"id,pk"`
	GroupID     snowflake.ID `bun:"group_id"`
	Kind        string       `bun:"kind"`
	Description string       `bun:"description"`
	Amount      int64        `bun:"amount"`
	Currency    string       `bun:"currency"`
	SplitType   string       `bun:"split_type"`
	PaidBy      snowflake.ID `bun:"paid_by"`
	AddedBy     snowflake.ID `bun:"added_by"`
	Date        string       `bun:"date"`
	UpdatedAt   int64        `bun:"updated_at"`
	DeletedAt   *int64       `bun:"deleted_at"`

	Participants []*ExpenseParticipant `bun:"rel:has-many,join:id=expense_id"`

	bun.BaseModel `bun:"expenses,alias:e"`
}

// ExpenseParticipant is the expense participant database model.
type ExpenseParticipant struct {
	ExpenseID  snowflake.ID `bun:"expense_id,pk"`
	UserID     snowflake.ID `bun:"user_id,pk"`
	Share      int64        `bun:"share"`
	SplitValue string       `bun:"split_value"`

	bun.BaseModel `bun:"expense_participants,alias:ep"`
}

// InsertExpense inserts an expense with its participants.
func InsertExpense(ctx context.Context, db bun.IDB, e *domain.Expense) error {
	if err := validateExpense(e); err != nil {
		return err
	}

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := sqlite.WithErrorChecking(tx.NewInsert().Model(expenseFromDomain(e)).Exec(ctx)); err != nil {
			return err
		}
		return insertParticipants(ctx, tx, e)
	})
}

// UpdateExpense updates an expense and replaces its participants.
func UpdateExpense(ctx context.Context, db bun.IDB, e *domain.Expense) error {
	if err := validateExpense(e); err != nil {
		return err
	}

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := sqlite.WithErrorChecking(tx.NewUpdate().Model(expenseFromDomain(e)).
			Column("description", "amount", "currency", "split_type", "paid_by", "date", "updated_at").
			WherePK().
			Where("deleted_at IS NULL").
			Exec(ctx)); err != nil {
			return err
		}

		if _, err := tx.NewDelete().Model((*ExpenseParticipant)(nil)).
			Where("expense_id = ?", e.ID).
			Exec(ctx); err != nil {
			return sqlite.NormalizeError(err)
		}

		return insertParticipants(ctx, tx, e)
	})
}

// DeleteExpense soft deletes an expense.
func DeleteExpense(ctx context.Context, db bun.IDB, id snowflake.ID, now time.Time) error {
	return sqlite.WithErrorChecking(db.NewUpdate().Model((*Expense)(nil)).
		Set("deleted_at = ?", now.Unix()).
		Where("id = ?", id).
		Where("deleted_at IS NULL").
		Exec(ctx))
}

// GetGroupExpense returns a non-deleted expense of a group.
func GetGroupExpense(ctx context.Context, db bun.IDB, groupID, id snowflake.ID) (*domain.Expense, error) {
	m := &Expense{}
	if err := db.NewSelect().Model(m).
		Relation("Participants", orderParticipants).
		Where("e.id = ?", id).
		Where("e.group_id = ?", groupID).
		Where("e.deleted_at IS NULL").
		Scan(ctx); err != nil {
		return nil, sqlite.NormalizeError(err)
	}
	return expenseToDomain(m), nil
}

// ExpenseCursor is a keyset pagination cursor for expense lists.
type ExpenseCursor struct {
	Date string
	ID   snowflake.ID
}

// ListGroupExpenses lists a group's non-deleted expenses, newest first.
// If after is not nil, only expenses after the cursor are returned.
func ListGroupExpenses(ctx context.Context, db bun.IDB, groupID snowflake.ID, after *ExpenseCursor, limit int) ([]*domain.Expense, error) {
	return listExpenses(ctx, db, after, limit, func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("e.group_id = ?", groupID)
	})
}

// ListUserExpenses lists non-deleted expenses the user paid or participates
// in across all their groups, newest first.
func ListUserExpenses(ctx context.Context, db bun.IDB, userID snowflake.ID, after *ExpenseCursor, limit int) ([]*domain.Expense, error) {
	return listExpenses(ctx, db, after, limit, func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("e.group_id IN (SELECT group_id FROM group_members WHERE user_id = ?)", userID).
			WhereGroup(" AND ", involves(userID))
	})
}

// ListUserExpensesWith lists non-deleted expenses in the user's groups that
// involve both the user and friend, newest first.
func ListUserExpensesWith(ctx context.Context, db bun.IDB, userID, friendID snowflake.ID, after *ExpenseCursor, limit int) ([]*domain.Expense, error) {
	return listExpenses(ctx, db, after, limit, func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("e.group_id IN (SELECT group_id FROM group_members WHERE user_id = ?)", userID).
			WhereGroup(" AND ", involves(userID)).
			WhereGroup(" AND ", involves(friendID))
	})
}

func involves(userID snowflake.ID) func(q *bun.SelectQuery) *bun.SelectQuery {
	return func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("e.paid_by = ?", userID).
			WhereOr("EXISTS (SELECT 1 FROM expense_participants p WHERE p.expense_id = e.id AND p.user_id = ? AND p.share > 0)", userID)
	}
}

func listExpenses(
	ctx context.Context,
	db bun.IDB,
	after *ExpenseCursor,
	limit int,
	filter func(*bun.SelectQuery) *bun.SelectQuery,
) ([]*domain.Expense, error) {
	var models []*Expense

	q := db.NewSelect().Model(&models).Apply(filter).Relation("Participants", orderParticipants).
		Where("e.deleted_at IS NULL").
		OrderExpr("e.date DESC, e.id DESC").
		Limit(int64(limit))

	if after != nil {
		q = q.Where("(e.date, e.id) < (?, ?)", after.Date, after.ID)
	}

	if err := q.Scan(ctx); err != nil {
		return nil, sqlite.NormalizeError(err)
	}

	return lo.Map(models, func(m *Expense, _ int) *domain.Expense { return expenseToDomain(m) }), nil
}

// CountUserExpensesInGroup counts non-deleted expenses in group that involve user.
func CountUserExpensesInGroup(ctx context.Context, db bun.IDB, groupID, userID snowflake.ID) (int64, error) {
	n, err := db.NewSelect().Model((*Expense)(nil)).
		Where("e.group_id = ?", groupID).
		Where("e.deleted_at IS NULL").
		WhereGroup(" AND ", involves(userID)).
		Count(ctx)
	return n, sqlite.NormalizeError(err)
}

// GroupSpending returns total non-settlement spending per currency.
func GroupSpending(ctx context.Context, db bun.IDB, groupID snowflake.ID) (map[string]int64, error) {
	var rows []struct {
		Currency string `bun:"currency"`
		Total    int64  `bun:"total"`
	}

	if err := db.NewSelect().Model((*Expense)(nil)).
		ColumnExpr("e.currency, SUM(e.amount) AS total").
		Where("e.group_id = ?", groupID).
		Where("e.deleted_at IS NULL").
		Where("e.kind = ?", domain.KindExpense).
		GroupExpr("e.currency").
		Scan(ctx, &rows); err != nil {
		return nil, sqlite.NormalizeError(err)
	}

	result := map[string]int64{}
	for _, r := range rows {
		result[r.Currency] = r.Total
	}
	return result, nil
}

func orderParticipants(q *bun.SelectQuery) *bun.SelectQuery {
	return q.Order("ep.user_id ASC")
}

func validateExpense(e *domain.Expense) error {
	if e.Amount <= 0 {
		return splitgo.InvalidValue.New("Amount must be positive")
	}

	var sum int64
	for _, p := range e.Participants {
		if p.Share < 0 {
			return splitgo.InvalidValue.New("Shares must not be negative")
		}
		sum += p.Share
	}

	if sum != e.Amount {
		return splitgo.InvalidValue.New("Shares must add up to the amount")
	}

	return nil
}

func insertParticipants(ctx context.Context, tx bun.Tx, e *domain.Expense) error {
	if len(e.Participants) == 0 {
		return splitgo.InvalidValue.New("Expense must have participants")
	}

	models := lo.Map(e.Participants, func(p domain.Participant, _ int) *ExpenseParticipant {
		return &ExpenseParticipant{
			ExpenseID:  e.ID,
			UserID:     p.UserID,
			Share:      p.Share,
			SplitValue: p.SplitValue,
		}
	})

	return sqlite.WithErrorChecking(tx.NewInsert().Model(&models).Exec(ctx))
}

func expenseFromDomain(e *domain.Expense) *Expense {
	return &Expense{
		ID:          e.ID,
		GroupID:     e.GroupID,
		Kind:        string(e.Kind),
		Description: e.Description,
		Amount:      e.Amount,
		Currency:    e.Currency,
		SplitType:   string(e.SplitType),
		PaidBy:      e.PaidBy,
		AddedBy:     e.AddedBy,
		Date:        e.Date.Format(domain.DateLayout),
		UpdatedAt:   e.UpdatedAt.Unix(),
	}
}

func expenseToDomain(m *Expense) *domain.Expense {
	date, err := time.Parse(domain.DateLayout, m.Date)
	if err != nil {
		panic(err)
	}

	return &domain.Expense{
		ID:          m.ID,
		GroupID:     m.GroupID,
		Kind:        domain.ExpenseKind(m.Kind),
		Description: m.Description,
		Amount:      m.Amount,
		Currency:    m.Currency,
		SplitType:   split.Type(m.SplitType),
		PaidBy:      m.PaidBy,
		AddedBy:     m.AddedBy,
		Date:        date,
		UpdatedAt:   time.Unix(m.UpdatedAt, 0),
		Participants: lo.Map(m.Participants, func(p *ExpenseParticipant, _ int) domain.Participant {
			return domain.Participant{UserID: p.UserID, Share: p.Share, SplitValue: p.SplitValue}
		}),
	}
}
