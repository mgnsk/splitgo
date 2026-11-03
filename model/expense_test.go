package model_test

import (
	"time"

	"github.com/mgnsk/splitgo"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/model"
	. "github.com/mgnsk/splitgo/pkg/testing"
	"github.com/mgnsk/splitgo/split"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("expenses", func() {
	var (
		alice, bob, carol *domain.User
		g                 *domain.Group
	)

	BeforeEach(func(ctx SpecContext) {
		alice = mustCreateUser(ctx, "alice@example.com")
		bob = mustCreateUser(ctx, "bob@example.com")
		carol = mustCreateUser(ctx, "carol@example.com")
		g = mustCreateGroup(ctx, "Trip", alice, bob, carol)
	})

	It("stores and loads an expense with participants", func(ctx SpecContext) {
		e := mustAddEqualExpense(ctx, g, alice, 10001, alice, bob, carol)

		loaded := Must(model.GetGroupExpense(ctx, db, g.ID, e.ID))
		Expect(loaded.Amount).To(Equal(int64(10001)))
		Expect(loaded.SplitType).To(Equal(split.Equal))
		Expect(loaded.Date.Format(domain.DateLayout)).To(Equal("2026-11-03"))
		Expect(loaded.Participants).To(ConsistOf(
			domain.Participant{UserID: alice.ID, Share: 3334},
			domain.Participant{UserID: bob.ID, Share: 3334},
			domain.Participant{UserID: carol.ID, Share: 3333},
		))
	})

	It("rejects shares that do not add up to the amount", func(ctx SpecContext) {
		e := newEqualExpense(g, alice, 1000, "EUR", alice, bob)
		e.Participants[0].Share++
		Expect(model.InsertExpense(ctx, db, e)).To(MatchError(splitgo.InvalidValue))
	})

	It("rejects non-positive amounts", func(ctx SpecContext) {
		e := newEqualExpense(g, alice, 1000, "EUR", alice, bob)
		e.Amount = 0
		e.Participants = []domain.Participant{{UserID: alice.ID, Share: 0}}
		Expect(model.InsertExpense(ctx, db, e)).To(MatchError(splitgo.InvalidValue))
	})

	It("does not find expenses of another group", func(ctx SpecContext) {
		other := mustCreateGroup(ctx, "Other", alice)
		e := mustAddEqualExpense(ctx, g, alice, 1000, alice, bob)

		_, err := model.GetGroupExpense(ctx, db, other.ID, e.ID)
		Expect(err).To(MatchError(splitgo.NotFound))
	})

	It("updates an expense replacing participants", func(ctx SpecContext) {
		e := mustAddEqualExpense(ctx, g, alice, 1000, alice, bob, carol)

		e.Amount = 2000
		e.PaidBy = bob.ID
		e.SplitType = split.Exact
		e.Participants = []domain.Participant{
			{UserID: alice.ID, Share: 1500, SplitValue: "15.00"},
			{UserID: bob.ID, Share: 500, SplitValue: "5.00"},
		}
		Expect(model.UpdateExpense(ctx, db, e)).To(Succeed())

		loaded := Must(model.GetGroupExpense(ctx, db, g.ID, e.ID))
		Expect(loaded.PaidBy).To(Equal(bob.ID))
		Expect(loaded.Participants).To(HaveExactElements(
			domain.Participant{UserID: alice.ID, Share: 1500, SplitValue: "15.00"},
			domain.Participant{UserID: bob.ID, Share: 500, SplitValue: "5.00"},
		))
	})

	It("rolls back a failed update", func(ctx SpecContext) {
		e := mustAddEqualExpense(ctx, g, alice, 1000, alice, bob)

		e.Description = "Changed"
		e.Participants = []domain.Participant{
			{UserID: alice.ID, Share: 500},
			{UserID: 12345, Share: 500}, // unknown user violates FK
		}
		Expect(model.UpdateExpense(ctx, db, e)).NotTo(Succeed())

		loaded := Must(model.GetGroupExpense(ctx, db, g.ID, e.ID))
		Expect(loaded.Description).To(Equal("Dinner"))
		Expect(loaded.Participants).To(HaveLen(2))
	})

	It("soft deletes expenses", func(ctx SpecContext) {
		e := mustAddEqualExpense(ctx, g, alice, 1000, alice, bob)
		Expect(model.DeleteExpense(ctx, db, e.ID, time.Now())).To(Succeed())

		_, err := model.GetGroupExpense(ctx, db, g.ID, e.ID)
		Expect(err).To(MatchError(splitgo.NotFound))
		Expect(model.DeleteExpense(ctx, db, e.ID, time.Now())).To(MatchError(splitgo.PreconditionFailed))
		Expect(model.UpdateExpense(ctx, db, e)).To(MatchError(splitgo.PreconditionFailed))
	})

	It("lists group expenses newest first with keyset pagination", func(ctx SpecContext) {
		var ids []any
		for i := range 5 {
			e := newEqualExpense(g, alice, 100, "EUR", alice, bob)
			e.Date = time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC)
			Expect(model.InsertExpense(ctx, db, e)).To(Succeed())
			ids = append([]any{HaveField("ID", e.ID)}, ids...)
		}

		page1 := Must(model.ListGroupExpenses(ctx, db, g.ID, nil, 3))
		Expect(page1).To(HaveExactElements(ids[:3]...))

		last := page1[len(page1)-1]
		page2 := Must(model.ListGroupExpenses(ctx, db, g.ID, &model.ExpenseCursor{
			Date: last.Date.Format(domain.DateLayout),
			ID:   last.ID,
		}, 3))
		Expect(page2).To(HaveExactElements(ids[3:]...))
	})

	It("lists only expenses involving the user", func(ctx SpecContext) {
		e1 := mustAddEqualExpense(ctx, g, alice, 1000, bob, carol) // alice paid, not participating
		e2 := mustAddEqualExpense(ctx, g, bob, 1000, bob, carol)   // alice not involved
		e3 := mustAddEqualExpense(ctx, g, carol, 1000, alice, carol)
		_ = e2

		Expect(Must(model.ListUserExpenses(ctx, db, alice.ID, nil, 10))).To(ConsistOf(
			HaveField("ID", e1.ID),
			HaveField("ID", e3.ID),
		))
		Expect(model.CountUserExpensesInGroup(ctx, db, g.ID, alice.ID)).To(Equal(int64(2)))
	})

	It("sums group spending per currency excluding settlements", func(ctx SpecContext) {
		mustAddEqualExpense(ctx, g, alice, 1000, alice, bob)
		mustAddEqualExpense(ctx, g, bob, 500, alice, bob)
		Expect(model.InsertExpense(ctx, db, newEqualExpense(g, bob, 700, "USD", alice, bob))).To(Succeed())
		mustSettle(ctx, g, bob, alice, 250)

		Expect(model.GroupSpending(ctx, db, g.ID)).To(Equal(map[string]int64{"EUR": 1500, "USD": 700}))
	})
})
