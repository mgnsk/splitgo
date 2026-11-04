package html

import (
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/pkg/snowflake"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
)

// ActivityMain renders recent expenses across all of my groups and friends.
// labels names the groups by ID.
func ActivityMain(me *domain.User, expenses []*domain.Expense, labels map[snowflake.ID]string, people People) Node {
	return Div(Class("stack"),
		H1(Text("Activity")),
		activityList(me, expenses, labels, people),
	)
}

func activityList(me *domain.User, expenses []*domain.Expense, labels map[snowflake.ID]string, people People) Node {
	return Div(Class("card card-flush"),
		Ul(Class("list"),
			If(len(expenses) == 0, emptyItem("Nothing here yet.")),
			Map(expenses, func(e *domain.Expense) Node {
				href := groupURL(e.GroupID) + "/expenses/" + e.ID.String()
				return Li(expenseItem(e, me.ID, people, href, labels[e.GroupID]))
			}),
		),
	)
}
