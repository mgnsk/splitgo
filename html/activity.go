package html

import (
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/pkg/snowflake"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
)

// ActivityMain renders recent expenses across all of my groups.
func ActivityMain(me *domain.User, expenses []*domain.Expense, groups map[snowflake.ID]*domain.Group, people People) Node {
	return Div(Class("stack"),
		H1(Text("Activity")),
		Div(Class("card card-flush"),
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
		),
	)
}
