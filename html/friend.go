package html

import (
	"maps"
	"net/url"

	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/html/components"
	"github.com/mgnsk/splitgo/money"
	"github.com/mgnsk/splitgo/pkg/snowflake"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
)

// FriendItem is a friend with my net balance per currency.
type FriendItem struct {
	User          *domain.User
	DirectGroupID snowflake.ID
	Net           map[string]int64
}

// FriendData is the data for a friend page.
type FriendData struct {
	Me     *domain.User
	Friend *domain.User
	// DirectGroupID holds our non-group expenses; zero if not friends.
	DirectGroupID snowflake.ID
	// CanRemove is set when the non-group expenses are settled.
	CanRemove bool
	// Balances are the totals with the friend per currency.
	Balances []domain.Balance
	// Debts are the debts between us per group and currency.
	Debts []domain.Debt
	// Labels names groups by ID.
	Labels   map[snowflake.ID]string
	Expenses []*domain.Expense
	People   People
}

func (d FriendData) newExpenseURL() string {
	return groupURL(d.DirectGroupID) + "/expenses/new"
}

// FriendMain renders the balance and shared expenses with a friend.
func FriendMain(d FriendData) Node {
	name := d.Friend.DisplayName()

	// The friend's name says nothing on their own page.
	labels := maps.Clone(d.Labels)
	delete(labels, d.DirectGroupID)

	var action Node
	if d.DirectGroupID != 0 {
		action = A(Class("btn btn-primary"), Href(d.newExpenseURL()), Text("Add expense"))
	} else {
		action = Form(Method("post"), Action("/friends"),
			Input(Type("hidden"), Name("email"), Value(d.Friend.Email)),
			Button(Type("submit"), Class("btn btn-primary"), Text("Add friend")),
		)
	}

	return Div(Class("stack"),
		Div(Class("page-head"),
			Div(Class("grow"),
				H1(Text(name)),
				P(Class("sub"), Text(d.Friend.Email)),
			),
			action,
		),

		Div(Class("card stack-sm"),
			If(len(d.Balances) == 0, P(Class("muted"), Text("You're settled up."))),
			Map(d.Balances, func(b domain.Balance) Node {
				return Div(Class("row-between"), Span(friendSentence(name, b.Amount)), Amount(b.Amount, b.Currency))
			}),
		),

		If(len(d.Debts) > 0, Group{
			H2(Class("section-title"), Text("Settle up")),
			Div(Class("card card-flush"),
				Ul(Class("list"),
					Map(d.Debts, func(debt domain.Debt) Node {
						label := d.Labels[debt.GroupID]
						if debt.GroupID == d.DirectGroupID {
							label = "You and " + name
						}
						amount := debt.Amount
						if debt.From == d.Me.ID {
							amount = -amount
						}
						return Li(Div(Class("list-item"),
							Div(Class("grow"),
								Div(Class("title"), Text(label)),
								Div(Class("muted small"), friendSentence(name, amount), Text(" "), Text(Fmt(debt.Amount, debt.Currency))),
							),
							A(Class("btn"), Href(settleURL(debt)), Text("Settle up")),
						))
					}),
				),
			),
		}),

		H2(Class("section-title"), Text("Activity")),
		activityList(d.Me, d.Expenses, labels, d.People),

		If(d.CanRemove, Form(Class("stack-sm"), Method("post"), Action(FriendURL(d.Friend.ID)+"/remove"),
			P(Class("hint"), Text("Removing a friend deletes your non-group expenses with them. Shared groups are not affected.")),
			Div(Button(Type("submit"), Class("btn btn-danger"), Text("Remove friend"))),
		)),
	)
}

func friendSentence(name string, amount int64) Node {
	if amount < 0 {
		return Textf("You owe %s", name)
	}
	return Textf("%s owes you", name)
}

// settleURL returns the record payment form prefilled to settle debt.
func settleURL(debt domain.Debt) string {
	c, ok := money.LookupCurrency(debt.Currency)
	if !ok {
		c = money.Currency{Code: debt.Currency, Decimals: 2}
	}
	q := url.Values{
		"from":     {debt.From.String()},
		"to":       {debt.To.String()},
		"amount":   {money.FormatPlain(debt.Amount, c)},
		"currency": {debt.Currency},
	}
	return groupURL(debt.GroupID) + "/settle?" + q.Encode()
}

// AddFriendMain renders the add friend form.
func AddFriendMain(email string, errs url.Values) Node {
	return Div(Class("stack"),
		H1(Text("Add friend")),
		Form(Class("card stack"), Method("post"), Action("/friends"),
			components.Field(components.FieldProps{
				Label:       "Email",
				Name:        "email",
				Type:        "email",
				Value:       email,
				Placeholder: "friend@example.com",
				Hint:        "They get an email to sign in and see your shared expenses.",
				Errors:      errs,
			}),
			Div(Class("row"),
				components.SubmitButton("Add friend", false),
				A(Class("btn"), Href("/"), Text("Cancel")),
			),
		),
	)
}

// ChooseLedgerMain renders the choice of a friend or group to add an
// expense with.
func ChooseLedgerMain(friends []FriendItem, groups []*domain.Group) Node {
	return Div(Class("stack"),
		H1(Text("Add expense")),
		P(Class("muted"), Text("With you and:")),

		H2(Class("section-title"), Text("Friends")),
		Div(Class("card card-flush"),
			Ul(Class("list"),
				Map(friends, func(f FriendItem) Node {
					return Li(A(Class("list-item"), Href(groupURL(f.DirectGroupID)+"/expenses/new"),
						Avatar(f.User.DisplayName()),
						Div(Class("grow title"), Text(f.User.DisplayName())),
					))
				}),
				Li(A(Class("list-item"), Href("/friends/new"),
					Div(Class("grow title"), Text("+ Add friend")),
				)),
			),
		),

		H2(Class("section-title"), Text("Groups")),
		Div(Class("card card-flush"),
			Ul(Class("list"),
				Map(groups, func(g *domain.Group) Node {
					return Li(A(Class("list-item"), Href(groupURL(g.ID)+"/expenses/new"),
						Avatar(g.Name),
						Div(Class("grow title"), Text(g.Name)),
					))
				}),
				Li(A(Class("list-item"), Href("/groups/new"),
					Div(Class("grow title"), Text("+ New group")),
				)),
			),
		),
	)
}
