// Package balance computes who owes whom from expense debts.
//
// All functions operate on a single currency; callers group debts by currency
// (and group) before calling them. Amounts are integer minor units.
package balance

import (
	"cmp"
	"slices"
)

// Debt means From owes To Amount.
type Debt[ID cmp.Ordered] struct {
	From   ID
	To     ID
	Amount int64
}

// FromExpense returns the debts created by an expense: every participant
// other than the payer owes the payer their share. Zero shares are skipped.
func FromExpense[ID cmp.Ordered](payer ID, shares map[ID]int64) []Debt[ID] {
	var debts []Debt[ID]
	for id, share := range shares {
		if id != payer && share != 0 {
			debts = append(debts, Debt[ID]{From: id, To: payer, Amount: share})
		}
	}
	sortDebts(debts)
	return debts
}

// Nets returns each person's net position: positive means they are owed
// money, negative means they owe. Zero positions are omitted. The values
// always sum to zero.
func Nets[ID cmp.Ordered](debts []Debt[ID]) map[ID]int64 {
	nets := map[ID]int64{}
	for _, d := range debts {
		if d.From == d.To {
			continue
		}
		nets[d.From] -= d.Amount
		nets[d.To] += d.Amount
	}
	for id, v := range nets {
		if v == 0 {
			delete(nets, id)
		}
	}
	return nets
}

// Pairwise nets debts between each pair of people, so that for every pair at
// most one debt remains, with a positive amount. Self-debts are dropped. The
// result is sorted by (From, To).
func Pairwise[ID cmp.Ordered](debts []Debt[ID]) []Debt[ID] {
	type pair struct{ a, b ID } // a < b

	sums := map[pair]int64{} // positive: a owes b
	for _, d := range debts {
		switch {
		case d.From == d.To:
			continue
		case d.From < d.To:
			sums[pair{d.From, d.To}] += d.Amount
		default:
			sums[pair{d.To, d.From}] -= d.Amount
		}
	}

	var result []Debt[ID]
	for p, v := range sums {
		switch {
		case v > 0:
			result = append(result, Debt[ID]{From: p.a, To: p.b, Amount: v})
		case v < 0:
			result = append(result, Debt[ID]{From: p.b, To: p.a, Amount: -v})
		}
	}
	sortDebts(result)
	return result
}

// Simplify returns a minimal-ish set of transfers that settles the given net
// positions (as returned by Nets). It greedily matches the largest creditor
// with the largest debtor; ties are broken by ID so the result is
// deterministic. The result has at most n-1 debts for n non-zero positions
// and settles every position exactly.
func Simplify[ID cmp.Ordered](nets map[ID]int64) []Debt[ID] {
	type entry struct {
		id     ID
		amount int64 // absolute value
	}

	var creditors, debtors []entry
	for id, v := range nets {
		switch {
		case v > 0:
			creditors = append(creditors, entry{id, v})
		case v < 0:
			debtors = append(debtors, entry{id, -v})
		}
	}

	byLargest := func(a, b entry) int {
		return cmp.Or(cmp.Compare(b.amount, a.amount), cmp.Compare(a.id, b.id))
	}

	var result []Debt[ID]
	for len(creditors) > 0 && len(debtors) > 0 {
		slices.SortFunc(creditors, byLargest)
		slices.SortFunc(debtors, byLargest)

		c, d := &creditors[0], &debtors[0]
		amount := min(c.amount, d.amount)
		result = append(result, Debt[ID]{From: d.id, To: c.id, Amount: amount})

		c.amount -= amount
		d.amount -= amount

		if c.amount == 0 {
			creditors = creditors[1:]
		}
		if d.amount == 0 {
			debtors = debtors[1:]
		}
	}

	sortDebts(result)
	return result
}

// Settle returns the debts to show for a set of raw debts: simplified
// transfers if simplify is set, otherwise pairwise netted debts.
func Settle[ID cmp.Ordered](debts []Debt[ID], simplify bool) []Debt[ID] {
	if simplify {
		return Simplify(Nets(debts))
	}
	return Pairwise(debts)
}

func sortDebts[ID cmp.Ordered](debts []Debt[ID]) {
	slices.SortFunc(debts, func(a, b Debt[ID]) int {
		return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To))
	})
}
