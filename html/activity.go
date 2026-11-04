package html

import (
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/pkg/snowflake"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
)

// FriendActivityMain renders the expenses shared with a friend and the
// balance with them.
func FriendActivityMain(
	me, friend *domain.User,
	balances []domain.Balance,
	expenses []*domain.Expense,
	groups map[snowflake.ID]*domain.Group,
	people People,
) Node {
	return Div(Class("stack"),
		Div(Class("page-head"),
			Div(
				H1(Text(friend.DisplayName())),
				P(Class("sub"), A(Href("/activity"), Text("All activity"))),
			),
		),
		Div(Class("card stack-sm"),
			If(len(balances) == 0, P(Class("muted"), Text("You're settled up."))),
			Map(balances, func(b domain.Balance) Node {
				sentence := Textf("%s owes you", friend.DisplayName())
				if b.Amount < 0 {
					sentence = Textf("You owe %s", friend.DisplayName())
				}
				return Div(Class("row-between"), Span(sentence), Amount(b.Amount, b.Currency))
			}),
		),
		activityList(me, expenses, groups, people),
	)
}

// ActivityMain renders recent expenses across all of my groups.
func ActivityMain(me *domain.User, expenses []*domain.Expense, groups map[snowflake.ID]*domain.Group, people People) Node {
	return Div(Class("stack"),
		H1(Text("Activity")),
		activityList(me, expenses, groups, people),
	)
}

func activityList(me *domain.User, expenses []*domain.Expense, groups map[snowflake.ID]*domain.Group, people People) Node {
	return Div(Class("card card-flush"),
		Ul(Class("list"),
			If(len(expenses) == 0, emptyItem("Nothing here yet.")),
			Map(expenses, func(e *domain.Expense) Node {
				groupName := ""
				if g, ok := groups[e.GroupID]; ok {
					groupName = g.Name
				}
				href := groupURL(e.GroupID) + "/expenses/" + e.ID.String()
				return Li(expenseItem(e, me.ID, people, href, groupName))
			}),
		),
	)
}
