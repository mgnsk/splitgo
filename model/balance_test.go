package model_test

import (
	"time"

	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/model"
	. "github.com/mgnsk/webkit/testutil"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("balances", func() {
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

	groupDebts := func(ctx SpecContext) []debt {
		debts, err := model.GroupDebts(ctx, db, Must(model.GetGroup(ctx, db, g.ID)))
		Expect(err).NotTo(HaveOccurred())
		return compact(debts)
	}

	It("is empty without expenses", func(ctx SpecContext) {
		Expect(groupDebts(ctx)).To(BeEmpty())
	})

	It("computes debts from an equal split", func(ctx SpecContext) {
		mustAddEqualExpense(ctx, g, alice, 3000, alice, bob, carol)

		Expect(groupDebts(ctx)).To(ConsistOf(
			debt{bob.ID, alice.ID, 1000, "EUR"},
			debt{carol.ID, alice.ID, 1000, "EUR"},
		))
	})

	It("nets debts between two people", func(ctx SpecContext) {
		mustAddEqualExpense(ctx, g, alice, 1000, alice, bob) // bob owes 500
		mustAddEqualExpense(ctx, g, bob, 600, alice, bob)    // alice owes 300

		Expect(groupDebts(ctx)).To(ConsistOf(debt{bob.ID, alice.ID, 200, "EUR"}))
	})

	It("keeps currencies separate", func(ctx SpecContext) {
		mustAddEqualExpense(ctx, g, alice, 1000, alice, bob)
		Expect(model.InsertExpense(ctx, db, newEqualExpense(g, bob, 1000, "USD", alice, bob))).To(Succeed())

		Expect(groupDebts(ctx)).To(ConsistOf(
			debt{bob.ID, alice.ID, 500, "EUR"},
			debt{alice.ID, bob.ID, 500, "USD"},
		))
	})

	It("ignores deleted expenses and reflects edits", func(ctx SpecContext) {
		e1 := mustAddEqualExpense(ctx, g, alice, 1000, alice, bob)
		e2 := mustAddEqualExpense(ctx, g, alice, 2000, alice, bob)

		Expect(model.DeleteExpense(ctx, db, e1.ID, time.Now())).To(Succeed())
		Expect(groupDebts(ctx)).To(ConsistOf(debt{bob.ID, alice.ID, 1000, "EUR"}))

		e2.Participants = []domain.Participant{{UserID: bob.ID, Share: 2000}}
		Expect(model.UpdateExpense(ctx, db, e2)).To(Succeed())
		Expect(groupDebts(ctx)).To(ConsistOf(debt{bob.ID, alice.ID, 2000, "EUR"}))
	})

	It("settles partially and fully", func(ctx SpecContext) {
		mustAddEqualExpense(ctx, g, alice, 2400, alice, bob) // bob owes 1200

		mustSettle(ctx, g, bob, alice, 600)
		Expect(groupDebts(ctx)).To(ConsistOf(debt{bob.ID, alice.ID, 600, "EUR"}))

		mustSettle(ctx, g, bob, alice, 600)
		Expect(groupDebts(ctx)).To(BeEmpty())

		// Overpaying flips the debt.
		mustSettle(ctx, g, bob, alice, 100)
		Expect(groupDebts(ctx)).To(ConsistOf(debt{alice.ID, bob.ID, 100, "EUR"}))
	})

	It("simplifies a chain when enabled", func(ctx SpecContext) {
		mustAddEqualExpense(ctx, g, bob, 1000, alice) // alice owes bob 10
		mustAddEqualExpense(ctx, g, carol, 1000, bob) // bob owes carol 10

		Expect(groupDebts(ctx)).To(ConsistOf(
			debt{alice.ID, bob.ID, 1000, "EUR"},
			debt{bob.ID, carol.ID, 1000, "EUR"},
		))

		g.SimplifyDebts = true
		Expect(model.UpdateGroup(ctx, db, g)).To(Succeed())
		Expect(groupDebts(ctx)).To(ConsistOf(debt{alice.ID, carol.ID, 1000, "EUR"}))

		// Settling the simplified debt clears everything.
		mustSettle(ctx, g, alice, carol, 1000)
		Expect(groupDebts(ctx)).To(BeEmpty())
	})

	It("aggregates user balances across groups", func(ctx SpecContext) {
		g2 := mustCreateGroup(ctx, "Flat", alice, bob)
		mustAddEqualExpense(ctx, g, alice, 1000, alice, bob)  // bob owes alice 5 in g
		mustAddEqualExpense(ctx, g2, alice, 3000, alice, bob) // bob owes alice 15 in g2
		mustAddEqualExpense(ctx, g, carol, 900, alice, bob, carol)

		groups := Must(model.ListUserGroups(ctx, db, alice.ID))
		debts := Must(model.UserDebts(ctx, db, alice.ID, groups))
		balances := domain.Balances(alice.ID, debts)

		Expect(balances).To(ConsistOf(
			domain.Balance{UserID: bob.ID, Currency: "EUR", Amount: 2000},
			domain.Balance{UserID: carol.ID, Currency: "EUR", Amount: -300},
		))

		owed, owes := domain.Totals(balances)
		Expect(owed).To(Equal(map[string]int64{"EUR": 2000}))
		Expect(owes).To(Equal(map[string]int64{"EUR": 300}))
	})
})
