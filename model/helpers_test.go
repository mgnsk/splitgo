package model_test

import (
	"context"
	"time"

	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/model"
	"github.com/mgnsk/splitgo/pkg/snowflake"
	"github.com/mgnsk/splitgo/split"
	. "github.com/onsi/gomega"
	"github.com/shopspring/decimal"
)

func mustCreateUser(ctx context.Context, email string) *domain.User {
	u := &domain.User{ID: snowflake.Generate(), Email: email, Currency: "EUR"}
	Expect(model.InsertUser(ctx, db, u)).To(Succeed())
	return u
}

func mustCreateGroup(ctx context.Context, name string, creator *domain.User, members ...*domain.User) *domain.Group {
	g := &domain.Group{
		ID:         snowflake.Generate(),
		Name:       name,
		Currency:   "EUR",
		InviteCode: snowflake.Generate().String(),
		CreatedBy:  creator.ID,
	}
	Expect(model.InsertGroup(ctx, db, g)).To(Succeed())
	for _, m := range members {
		Expect(model.AddGroupMember(ctx, db, g.ID, m.ID)).To(Succeed())
	}
	return g
}

// newEqualExpense builds an expense paid by payer, split equally.
func newEqualExpense(g *domain.Group, payer *domain.User, amount int64, currency string, participants ...*domain.User) *domain.Expense {
	shares, err := split.Compute(amount, split.Equal, make([]decimal.Decimal, len(participants)))
	Expect(err).NotTo(HaveOccurred())

	e := &domain.Expense{
		ID:          snowflake.Generate(),
		GroupID:     g.ID,
		Kind:        domain.KindExpense,
		Description: "Dinner",
		Amount:      amount,
		Currency:    currency,
		SplitType:   split.Equal,
		PaidBy:      payer.ID,
		AddedBy:     payer.ID,
		Date:        time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Now(),
	}
	for i, p := range participants {
		e.Participants = append(e.Participants, domain.Participant{UserID: p.ID, Share: shares[i]})
	}
	return e
}

func mustAddEqualExpense(ctx context.Context, g *domain.Group, payer *domain.User, amount int64, participants ...*domain.User) *domain.Expense {
	e := newEqualExpense(g, payer, amount, "EUR", participants...)
	Expect(model.InsertExpense(ctx, db, e)).To(Succeed())
	return e
}

// mustSettle records that from paid to amount.
func mustSettle(ctx context.Context, g *domain.Group, from, to *domain.User, amount int64) *domain.Expense {
	e := &domain.Expense{
		ID:           snowflake.Generate(),
		GroupID:      g.ID,
		Kind:         domain.KindSettlement,
		Description:  "Settle up",
		Amount:       amount,
		Currency:     "EUR",
		SplitType:    split.Exact,
		PaidBy:       from.ID,
		AddedBy:      from.ID,
		Date:         time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Now(),
		Participants: []domain.Participant{{UserID: to.ID, Share: amount}},
	}
	Expect(model.InsertExpense(ctx, db, e)).To(Succeed())
	return e
}

// debt is a compact debt for assertions.
type debt struct {
	From, To snowflake.ID
	Amount   int64
	Currency string
}

func compact(debts []domain.Debt) []debt {
	out := []debt{}
	for _, d := range debts {
		out = append(out, debt{d.From, d.To, d.Amount, d.Currency})
	}
	return out
}
