package html

import (
	"fmt"
	"net/url"

	"github.com/mgnsk/splitgo/contract"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/html/components"
	"github.com/mgnsk/splitgo/money"
	"github.com/mgnsk/splitgo/pkg/snowflake"
	. "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	. "maragu.dev/gomponents/components"
	. "maragu.dev/gomponents/html"
)

// GroupData is the common data of group pages.
type GroupData struct {
	Me      *domain.User
	Group   *domain.Group
	Members []*domain.User
	People  People
}

func groupURL(id snowflake.ID) string {
	return fmt.Sprintf("/groups/%s", id)
}

func groupHeader(d GroupData, tab string) Node {
	base := groupURL(d.Group.ID)

	tabLink := func(href, text, name string) Node {
		return A(Href(href), Text(text), Classes{"active": tab == name})
	}

	return Div(Class("stack"),
		Div(Class("page-head"),
			Div(Class("grow"),
				H1(Text(d.Group.Name)),
				P(Class("sub"), Text(pluralize(len(d.Members), "member", "members"))),
			),
			A(Class("btn btn-primary"), Href(base+"/expenses/new"), Text("Add expense")),
		),
		Nav(Class("tabs"),
			tabLink(base, "Expenses", "expenses"),
			tabLink(base+"/balances", "Balances", "balances"),
			tabLink(base+"/settings", "Settings", "settings"),
		),
	)
}

// GroupExpensesMain renders the group expenses page.
func GroupExpensesMain(d GroupData, expenses []*domain.Expense, more bool) Node {
	return Div(Class("stack"),
		groupHeader(d, "expenses"),
		Div(Class("card card-flush"),
			Ul(Class("list"), ID("expense-list"),
				If(len(expenses) == 0, Li(Div(Class("list-empty"),
					Text("No expenses yet. "),
					A(Href(groupURL(d.Group.ID)+"/expenses/new"), Text("Add the first one")),
					Text("."),
				))),
				ExpenseListPartial(d, expenses, more),
			),
		),
	)
}

// ExpenseListPartial renders expense list items with a lazy "load more" item.
func ExpenseListPartial(d GroupData, expenses []*domain.Expense, more bool) Node {
	return Group{
		Map(expenses, func(e *domain.Expense) Node {
			return Li(expenseItem(e, d.Me.ID, d.People, groupURL(d.Group.ID)+"/expenses/"+e.ID.String(), ""))
		}),
		Iff(more && len(expenses) > 0, func() Node {
			last := expenses[len(expenses)-1]
			q := url.Values{
				"after_date": {last.Date.Format(domain.DateLayout)},
				"after_id":   {last.ID.String()},
			}
			return Li(
				hx.Get(groupURL(d.Group.ID)+"?"+q.Encode()),
				hx.Trigger("revealed"),
				hx.Swap("outerHTML"),
				Div(Class("list-empty"), Text("Loading…")),
			)
		}),
	}
}

// expenseItem renders an expense row. context is prepended to the payer line.
func expenseItem(e *domain.Expense, me snowflake.ID, people People, href, context string) Node {
	var (
		line  string
		right Node
	)

	if e.Kind == domain.KindSettlement {
		to := snowflake.ID(0)
		if len(e.Participants) > 0 {
			to = e.Participants[0].UserID
		}
		line = fmt.Sprintf("%s paid %s %s", people.Name(e.PaidBy, me), lower(people.Name(to, me)), Fmt(e.Amount, e.Currency))
		switch me {
		case e.PaidBy:
			right = Div(Class("right small"), Div(Class("muted"), Text("you paid")), Amount(e.Amount, e.Currency))
		case to:
			right = Div(Class("right small"), Div(Class("muted"), Text("you received")), Amount(-e.Amount, e.Currency))
		}
	} else {
		line = fmt.Sprintf("%s paid %s", people.Name(e.PaidBy, me), Fmt(e.Amount, e.Currency))
		net := e.Net(me)
		switch {
		case net > 0:
			right = Div(Class("right small"), Div(Class("muted"), Text("you lent")), Amount(net, e.Currency))
		case net < 0:
			right = Div(Class("right small"), Div(Class("muted"), Text("you borrowed")), Amount(net, e.Currency))
		default:
			right = Div(Class("right small muted"), Text("not involved"))
		}
	}

	if context != "" {
		line = context + " · " + line
	}

	title := e.Description
	if e.Kind == domain.KindSettlement {
		title = "Payment"
	}

	return A(Class("list-item"), Href(href),
		dateBadge(e),
		Div(Class("grow"),
			Div(Class("title"), Text(title)),
			Div(Class("muted small"), Text(line)),
		),
		right,
	)
}

func lower(name string) string {
	if name == "You" {
		return "you"
	}
	return name
}

// GroupBalancesMain renders the group balances page.
func GroupBalancesMain(d GroupData, debts []domain.Debt, spending map[string]int64) Node {
	base := groupURL(d.Group.ID)

	return Div(Class("stack"),
		groupHeader(d, "balances"),

		Div(Class("row-between"),
			H2(Class("section-title"), Text("Who owes whom")),
			A(Class("btn btn-sm"), Href(base+"/settle"), Text("Record a payment")),
		),
		Div(Class("card card-flush"),
			Ul(Class("list"),
				If(len(debts) == 0, emptyItem("Everyone is settled up.")),
				Map(debts, func(debt domain.Debt) Node {
					q := url.Values{
						"from":     {debt.From.String()},
						"to":       {debt.To.String()},
						"currency": {debt.Currency},
						"amount":   {money.FormatPlain(debt.Amount, money.MustCurrency(debt.Currency))},
					}
					involved := debt.From == d.Me.ID || debt.To == d.Me.ID
					return Li(Div(Class("list-item"),
						Avatar(d.People.name(debt.From)),
						Div(Class("grow"),
							Div(Class("title"),
								Text(d.People.Name(debt.From, d.Me.ID)), Text(owesVerb(debt.From, d.Me.ID)),
								Text(lower(d.People.Name(debt.To, d.Me.ID))),
							),
							Div(Class("small"), If(involved, Amount(signFor(debt, d.Me.ID), debt.Currency)),
								If(!involved, Span(Class("muted"), Text(Fmt(debt.Amount, debt.Currency))))),
						),
						A(Classes{"btn": true, "btn-sm": true, "btn-primary": involved},
							Href(base+"/settle?"+q.Encode()), Text("Settle up")),
					))
				}),
			),
		),
		If(d.Group.SimplifyDebts, P(Class("hint"), Text("Debts are simplified to minimize the number of payments."))),

		If(len(spending) > 0, Group{
			H2(Class("section-title"), Text("Total spending")),
			Div(Class("card stack-sm"),
				Map(sortedCurrencies(spending), func(code string) Node {
					return Div(Class("row-between"),
						Span(Class("muted"), Text(code)),
						Strong(Text(Fmt(spending[code], code))),
					)
				}),
			),
		}),
	)
}

func owesVerb(from, me snowflake.ID) string {
	if from == me {
		return " owe "
	}
	return " owes "
}

// signFor returns the debt amount signed from my point of view.
func signFor(d domain.Debt, me snowflake.ID) int64 {
	if d.From == me {
		return -d.Amount
	}
	return d.Amount
}

// GroupSettingsData is the data of the group settings page.
type GroupSettingsData struct {
	GroupData
	Form      contract.GroupForm
	Errors    url.Values
	AddEmail  string
	AddErrors url.Values
	InviteURL string
	CanLeave  bool
	CanDelete bool
}

// GroupSettingsMain renders the group settings page.
func GroupSettingsMain(d GroupSettingsData) Node {
	base := groupURL(d.Group.ID)

	return Div(Class("stack"),
		groupHeader(d.GroupData, "settings"),

		H2(Class("section-title"), Text("Members")),
		Div(Class("card card-flush"),
			Ul(Class("list"),
				Map(d.Members, func(u *domain.User) Node {
					return Li(Div(Class("list-item"),
						Avatar(u.DisplayName()),
						Div(Class("grow"),
							Div(Class("title"), Text(u.DisplayName()), If(u.ID == d.Me.ID, Span(Class("muted"), Text(" (you)")))),
							Div(Class("muted small"), Text(u.Email)),
						),
					))
				}),
			),
		),

		Form(Class("card stack"), Method("post"), Action(base+"/members"),
			components.Field(components.FieldProps{
				Label:       "Add someone by email",
				Name:        "email",
				Type:        "email",
				Value:       d.AddEmail,
				Placeholder: "friend@example.com",
				Hint:        "They'll get an email with a link to join.",
				Errors:      d.AddErrors,
				Required:    true,
			}),
			Div(components.SubmitButton("Add member", false)),
		),

		Div(Class("card stack-sm"),
			Label(For("invite-url"), Class("label"), Text("Invite link")),
			Input(ID("invite-url"), Type("text"), ReadOnly(), Value(d.InviteURL)),
			P(Class("hint"), Text("Anyone with this link can join the group after signing in.")),
		),

		H2(Class("section-title"), Text("Group settings")),
		Form(Class("card stack"), Method("post"), Action(base+"/settings"),
			components.Field(components.FieldProps{
				Label:    "Name",
				Name:     "name",
				Value:    d.Form.Name,
				Errors:   d.Errors,
				Required: true,
			}),
			CurrencySelect("Default currency", "currency", d.Form.Currency, d.Errors),
			Label(Class("check"),
				Input(Type("checkbox"), Name("simplify_debts"), Value("true"), If(d.Form.SimplifyDebts, Checked())),
				Span(
					Div(Text("Simplify debts")),
					Div(Class("hint"), Text("Combine debts so the group needs fewer payments to settle up.")),
				),
			),
			Div(components.SubmitButton("Save", false)),
		),

		Div(Class("card stack-sm"),
			Div(Class("row wrap"),
				Button(Type("button"), Class("btn btn-danger"), If(!d.CanLeave, Disabled()),
					hx.Post(base+"/leave"), hx.Confirm("Leave this group?"), Text("Leave group")),
				If(d.Group.CreatedBy == d.Me.ID,
					Button(Type("button"), Class("btn btn-danger"), If(!d.CanDelete, Disabled()),
						hx.Post(base+"/delete"), hx.Confirm("Delete this group and all its expenses?"), Text("Delete group")),
				),
			),
			P(Class("hint"), Text("You can leave or delete a group once all balances involving you are settled.")),
		),
	)
}

// CurrencySelect renders a currency select field.
func CurrencySelect(label, name, value string, errs url.Values, attrs ...Node) Node {
	var opts []components.SelectOption
	for _, c := range money.Currencies() {
		opts = append(opts, components.SelectOption{Value: c.Code, Label: c.Code})
	}
	return components.SelectField(components.SelectProps{
		Label:   label,
		Name:    name,
		Value:   value,
		Options: opts,
		Errors:  errs,
		Attrs:   attrs,
	})
}

// NewGroupMain renders the create group page.
func NewGroupMain(form contract.GroupForm, errs url.Values) Node {
	return Div(Class("stack"),
		H1(Text("New group")),
		Form(Class("card stack"), Method("post"), Action("/groups"),
			components.Field(components.FieldProps{
				Label:       "Name",
				Name:        "name",
				Value:       form.Name,
				Placeholder: "Trip to Lisbon, Flat 3B…",
				Errors:      errs,
				Required:    true,
				Attrs:       []Node{AutoFocus()},
			}),
			CurrencySelect("Default currency", "currency", form.Currency, errs),
			Div(components.SubmitButton("Create group", false)),
		),
	)
}

// JoinMain renders the join group page.
func JoinMain(group *domain.Group, memberCount int, code string) Node {
	return Form(Class("card stack"), Method("post"), Action("/join/"+code),
		Div(Class("stack-sm"),
			H1(Textf("Join %s", group.Name)),
			P(Class("muted"), Text(pluralize(memberCount, "member", "members"))),
		),
		components.SubmitButton("Join group", true),
	)
}
