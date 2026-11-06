package html

import (
	"cmp"
	"maps"
	"slices"

	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/money"
	"github.com/mgnsk/webkit/snowflake"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/components"
	. "maragu.dev/gomponents/html"
)

// People resolves user IDs to users for display.
type People map[snowflake.ID]*domain.User

// Name returns the display name of a user, or "You" for me.
func (p People) Name(id, me snowflake.ID) string {
	if id == me {
		return "You"
	}
	return p.name(id)
}

// name returns the display name of a user.
func (p People) name(id snowflake.ID) string {
	if u, ok := p[id]; ok {
		return u.DisplayName()
	}
	return "Former member"
}

// Fmt formats minor units of currency code.
func Fmt(minor int64, code string) string {
	c, ok := money.LookupCurrency(code)
	if !ok {
		c = money.Currency{Code: code, Symbol: code, Decimals: 2}
	}
	return money.Format(minor, c)
}

// Amount renders a colored amount: positive (owed to me) or negative (I owe).
func Amount(minor int64, code string) Node {
	abs := minor
	if abs < 0 {
		abs = -abs
	}
	return Span(Classes{"pos": minor > 0, "neg": minor < 0, "nowrap": true}, Text(Fmt(abs, code)))
}

// sortedCurrencies returns map keys sorted.
func sortedCurrencies(m map[string]int64) []string {
	return slices.Sorted(maps.Keys(m))
}

func dateBadge(e *domain.Expense) Node {
	return Div(Class("date-badge"),
		Text(e.Date.Format("Jan")),
		Strong(Text(e.Date.Format("2"))),
	)
}

func sortBalances(balances []domain.Balance, people People) {
	slices.SortFunc(balances, func(a, b domain.Balance) int {
		return cmp.Or(
			cmp.Compare(people.name(a.UserID), people.name(b.UserID)),
			cmp.Compare(a.UserID, b.UserID),
			cmp.Compare(a.Currency, b.Currency),
		)
	})
}

func emptyItem(text string) Node {
	return Li(Div(Class("list-empty"), Text(text)))
}
