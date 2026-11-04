package html

import (
	"github.com/mgnsk/splitgo/domain"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
)

// GroupSummary is a group with my net balance per currency.
type GroupSummary struct {
	Group *domain.Group
	Net   map[string]int64
}

// HomeData is the data for the home page.
type HomeData struct {
	Me      *domain.User
	Owed    map[string]int64
	Owes    map[string]int64
	Friends []FriendItem
	// Others are balances with people who are not friends, from groups.
	Others []domain.Balance
	Groups []GroupSummary
	People People
}

// HomeMain renders the home page.
func HomeMain(d HomeData) Node {
	sortBalances(d.Others, d.People)

	return Div(Class("stack"),
		Div(Class("page-head"),
			Div(
				H1(Textf("Hi, %s", d.Me.DisplayName())),
				P(Class("sub"), Text("Here's where you stand.")),
			),
			A(Class("btn btn-primary"), Href("/expenses/new"), Text("Add expense")),
		),

		Div(Class("summary"),
			summaryCard("You are owed", d.Owed, true),
			summaryCard("You owe", d.Owes, false),
		),
		If(len(d.Owed) == 0 && len(d.Owes) == 0, P(Class("hint"), Text("You're all settled up."))),

		H2(Class("section-title"), Text("Friends")),
		Div(Class("card card-flush"),
			Ul(Class("list"),
				Map(d.Friends, func(f FriendItem) Node {
					return Li(A(Class("list-item"), Href(FriendURL(f.User.ID)),
						Avatar(f.User.DisplayName()),
						Div(Class("grow"),
							Div(Class("title"), Text(f.User.DisplayName())),
							Div(Class("muted small"), netSummary(f.Net)),
						),
					))
				}),
				Li(A(Class("list-item"), Href("/friends/new"),
					Div(Class("grow title"), Text("+ Add friend")),
				)),
			),
		),

		If(len(d.Others) > 0, Group{
			H2(Class("section-title"), Text("Other balances")),
			Div(Class("card card-flush"),
				Ul(Class("list"),
					Map(d.Others, func(b domain.Balance) Node {
						return Li(A(Class("list-item"), Href(FriendURL(b.UserID)),
							Avatar(d.People.name(b.UserID)),
							Div(Class("grow"),
								Div(Class("title"), Text(d.People.name(b.UserID))),
								Div(Class("muted small"), balanceSentence(b)),
							),
							Amount(b.Amount, b.Currency),
						))
					}),
				),
			),
		}),

		H2(Class("section-title"), Text("Groups")),
		Div(Class("card card-flush"),
			Ul(Class("list"),
				Map(d.Groups, func(g GroupSummary) Node {
					return Li(A(Class("list-item"), Href(groupURL(g.Group.ID)),
						Avatar(g.Group.Name),
						Div(Class("grow"),
							Div(Class("title"), Text(g.Group.Name)),
							Div(Class("muted small"), netSummary(g.Net)),
						),
					))
				}),
				Li(A(Class("list-item"), Href("/groups/new"),
					Div(Class("grow title"), Text("+ New group")),
				)),
			),
		),
	)
}

func summaryCard(label string, totals map[string]int64, positive bool) Node {
	codes := sortedCurrencies(totals)

	return Div(Class("card"),
		Div(Class("label"), Text(label)),
		If(len(codes) == 0, Div(Class("value muted"), Text("—"))),
		Map(codes, func(code string) Node {
			v := totals[code]
			if !positive {
				v = -v
			}
			return Div(Class("value"), Amount(v, code))
		}),
	)
}

func balanceSentence(b domain.Balance) Node {
	if b.Amount > 0 {
		return Textf("owes you %s", Fmt(b.Amount, b.Currency))
	}
	return Textf("you owe %s", Fmt(-b.Amount, b.Currency))
}

func netSummary(net map[string]int64) Node {
	codes := sortedCurrencies(net)
	var parts Group
	for _, code := range codes {
		v := net[code]
		if v == 0 {
			continue
		}
		if len(parts) > 0 {
			parts = append(parts, Text(" · "))
		}
		if v > 0 {
			parts = append(parts, Text("you are owed "), Amount(v, code))
		} else {
			parts = append(parts, Text("you owe "), Amount(v, code))
		}
	}
	if len(parts) == 0 {
		return Text("settled up")
	}
	return parts
}
