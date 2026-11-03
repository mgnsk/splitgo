package balance_test

import (
	"math/rand/v2"
	"testing"

	"github.com/mgnsk/splitgo/balance"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type debt = balance.Debt[int]

const (
	alice = iota + 1
	bob
	carol
	dave
)

var _ = Describe("FromExpense", func() {
	It("makes every non-payer owe the payer their share", func() {
		Expect(balance.FromExpense(alice, map[int]int64{alice: 334, bob: 333, carol: 333})).To(Equal([]debt{
			{From: bob, To: alice, Amount: 333},
			{From: carol, To: alice, Amount: 333},
		}))
	})

	It("handles the payer not participating", func() {
		Expect(balance.FromExpense(alice, map[int]int64{bob: 1000})).To(Equal([]debt{
			{From: bob, To: alice, Amount: 1000},
		}))
	})

	It("skips zero shares", func() {
		Expect(balance.FromExpense(alice, map[int]int64{alice: 1000, bob: 0})).To(BeEmpty())
	})
})

var _ = Describe("Nets", func() {
	It("sums to zero and omits settled people", func() {
		nets := balance.Nets([]debt{
			{From: bob, To: alice, Amount: 500},
			{From: carol, To: alice, Amount: 300},
			{From: alice, To: carol, Amount: 300},
			{From: dave, To: dave, Amount: 999},
		})
		Expect(nets).To(Equal(map[int]int64{alice: 500, bob: -500}))
	})
})

var _ = Describe("Pairwise", func() {
	It("nets opposite debts", func() {
		Expect(balance.Pairwise([]debt{
			{From: bob, To: alice, Amount: 1000},
			{From: alice, To: bob, Amount: 300},
			{From: bob, To: alice, Amount: 50},
		})).To(Equal([]debt{{From: bob, To: alice, Amount: 750}}))
	})

	It("flips direction when the other side owes more", func() {
		Expect(balance.Pairwise([]debt{
			{From: alice, To: bob, Amount: 100},
			{From: bob, To: alice, Amount: 300},
		})).To(Equal([]debt{{From: bob, To: alice, Amount: 200}}))
	})

	It("drops settled pairs and self debts", func() {
		Expect(balance.Pairwise([]debt{
			{From: alice, To: bob, Amount: 100},
			{From: bob, To: alice, Amount: 100},
			{From: carol, To: carol, Amount: 100},
		})).To(BeEmpty())
	})

	It("keeps pairs separate and sorted", func() {
		Expect(balance.Pairwise([]debt{
			{From: carol, To: alice, Amount: 1},
			{From: bob, To: alice, Amount: 2},
			{From: alice, To: bob, Amount: 3},
		})).To(Equal([]debt{
			{From: alice, To: bob, Amount: 1},
			{From: carol, To: alice, Amount: 1},
		}))
	})
})

var _ = Describe("Simplify", func() {
	It("collapses a chain into one transfer", func() {
		// alice owes bob 10, bob owes carol 10 => alice pays carol 10.
		debts := []debt{
			{From: alice, To: bob, Amount: 1000},
			{From: bob, To: carol, Amount: 1000},
		}
		Expect(balance.Simplify(balance.Nets(debts))).To(Equal([]debt{
			{From: alice, To: carol, Amount: 1000},
		}))
	})

	It("removes a cycle entirely", func() {
		debts := []debt{
			{From: alice, To: bob, Amount: 500},
			{From: bob, To: carol, Amount: 500},
			{From: carol, To: alice, Amount: 500},
		}
		Expect(balance.Simplify(balance.Nets(debts))).To(BeEmpty())
	})

	It("matches largest creditor with largest debtor", func() {
		nets := map[int]int64{alice: 700, bob: 300, carol: -600, dave: -400}
		Expect(balance.Simplify(nets)).To(Equal([]debt{
			{From: carol, To: alice, Amount: 600},
			{From: dave, To: alice, Amount: 100},
			{From: dave, To: bob, Amount: 300},
		}))
	})

	It("returns nothing when everyone is settled", func() {
		Expect(balance.Simplify(map[int]int64{})).To(BeEmpty())
	})

	It("preserves nets with at most n-1 transfers on random graphs", func() {
		r := rand.New(rand.NewPCG(3, 4))
		for range 1000 {
			n := 2 + r.IntN(8)
			var debts []debt
			for range r.IntN(30) {
				from, to := 1+r.IntN(n), 1+r.IntN(n)
				debts = append(debts, debt{From: from, To: to, Amount: 1 + r.Int64N(100000)})
			}

			nets := balance.Nets(debts)
			simplified := balance.Simplify(nets)

			Expect(balance.Nets(simplified)).To(Equal(nets))
			if len(nets) > 0 {
				Expect(len(simplified)).To(BeNumerically("<=", len(nets)-1))
			}
			for _, d := range simplified {
				Expect(d.From).NotTo(Equal(d.To))
				Expect(d.Amount).To(BeNumerically(">", 0))
			}

			// Pairwise also preserves nets.
			Expect(balance.Nets(balance.Pairwise(debts))).To(Equal(nets))
		}
	})
})

var _ = Describe("Settle", func() {
	debts := []debt{
		{From: alice, To: bob, Amount: 1000},
		{From: bob, To: carol, Amount: 1000},
	}

	It("uses pairwise debts without simplification", func() {
		Expect(balance.Settle(debts, false)).To(Equal(balance.Pairwise(debts)))
	})

	It("uses simplified debts with simplification", func() {
		Expect(balance.Settle(debts, true)).To(Equal([]debt{{From: alice, To: carol, Amount: 1000}}))
	})
})

func FuzzSimplify(f *testing.F) {
	f.Add([]byte{1, 2, 10, 2, 3, 10})
	f.Add([]byte{1, 2, 5, 2, 1, 7, 3, 1, 200})

	f.Fuzz(func(t *testing.T, data []byte) {
		var debts []debt
		for i := 0; i+2 < len(data); i += 3 {
			debts = append(debts, debt{From: int(data[i] % 6), To: int(data[i+1] % 6), Amount: int64(data[i+2]) + 1})
		}

		nets := balance.Nets(debts)
		var sum int64
		for _, v := range nets {
			sum += v
		}
		if sum != 0 {
			t.Fatalf("nets do not sum to zero: %v", nets)
		}

		for _, settled := range [][]debt{balance.Simplify(nets), balance.Pairwise(debts)} {
			got := balance.Nets(settled)
			if len(got) != len(nets) {
				t.Fatalf("nets changed: %v vs %v", got, nets)
			}
			for id, v := range nets {
				if got[id] != v {
					t.Fatalf("nets changed: %v vs %v", got, nets)
				}
			}
		}
	})
}
