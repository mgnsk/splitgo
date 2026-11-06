package domain

import (
	"github.com/mgnsk/splitgo/balance"
	"github.com/mgnsk/webkit/snowflake"
)

// Debt means From owes To Amount minor units of Currency.
type Debt struct {
	balance.Debt[snowflake.ID]
	Currency string
	GroupID  snowflake.ID
}

// Balance is a user's balance with one other person in one currency.
// Positive Amount means the other person owes the user.
type Balance struct {
	UserID   snowflake.ID
	Currency string
	Amount   int64
}

// Balances aggregates debts into user's per-person, per-currency balances.
// Positive amounts mean the other person owes user. Zero balances are omitted.
func Balances(userID snowflake.ID, debts []Debt) []Balance {
	type key struct {
		other    snowflake.ID
		currency string
	}

	sums := map[key]int64{}
	var keys []key
	add := func(k key, v int64) {
		if _, ok := sums[k]; !ok {
			keys = append(keys, k)
		}
		sums[k] += v
	}

	for _, d := range debts {
		switch userID {
		case d.To:
			add(key{d.From, d.Currency}, d.Amount)
		case d.From:
			add(key{d.To, d.Currency}, -d.Amount)
		}
	}

	var result []Balance
	for _, k := range keys {
		if v := sums[k]; v != 0 {
			result = append(result, Balance{UserID: k.other, Currency: k.currency, Amount: v})
		}
	}
	return result
}

// Totals sums balances per currency into what user is owed and owes.
func Totals(balances []Balance) (owed, owes map[string]int64) {
	owed, owes = map[string]int64{}, map[string]int64{}
	for _, b := range balances {
		if b.Amount > 0 {
			owed[b.Currency] += b.Amount
		} else {
			owes[b.Currency] -= b.Amount
		}
	}
	return owed, owes
}
