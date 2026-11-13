package html

import (
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/mgnsk/splitgo/contract"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/html/components"
	"github.com/mgnsk/splitgo/money"
	"github.com/mgnsk/splitgo/split"
	"github.com/mgnsk/webkit/snowflake"
	"github.com/shopspring/decimal"
	. "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	. "maragu.dev/gomponents/html"
)

// ExpenseFormData is the data of the add/edit expense page.
type ExpenseFormData struct {
	GroupData
	// Participants are the people who can take part in the split, sorted by ID.
	Participants []*domain.User
	Form         *contract.ExpenseForm
	Errors       url.Values
	ExpenseID    snowflake.ID
}

func (d ExpenseFormData) splitURL(part string) string {
	return fmt.Sprintf("%s/expenses/split?part=%s", groupURL(d.Group.ID), part)
}

// ExpenseFormMain renders the add/edit expense page.
func ExpenseFormMain(d ExpenseFormData) Node {
	base := groupURL(d.Group.ID)
	action := base + "/expenses"
	title := "Add expense"
	if d.ExpenseID != 0 {
		action = base + "/expenses/" + d.ExpenseID.String()
		title = "Edit expense"
	}

	previewAttrs := func(trigger string) Node {
		return Group{
			hx.Post(d.splitURL("preview")),
			hx.Trigger(trigger),
			hx.Target("#split-preview"),
			hx.Swap("outerHTML"),
		}
	}

	return Div(Class("stack"),
		Div(Class("page-head"),
			Div(
				H1(Text(title)),
				P(Class("sub"), ledgerLink(d.GroupData)),
			),
		),
		Form(ID("expense-form"), Class("card stack"), Method("post"), Action(action),
			If(d.ExpenseID != 0, Input(Type("hidden"), Name("expense_id"), Value(d.ExpenseID.String()))),
			components.Field(components.FieldProps{
				Label:       "Description",
				Name:        "description",
				Value:       d.Form.Description,
				Placeholder: "Groceries, taxi, dinner…",
				Errors:      d.Errors,
				Required:    true,
				Attrs:       []Node{If(d.ExpenseID == 0, AutoFocus()), MaxLength("200")},
			}),
			Div(Class("grid-amount"),
				components.Field(components.FieldProps{
					Label:       "Amount",
					Name:        "amount",
					Value:       d.Form.Amount,
					Placeholder: "0.00",
					Errors:      d.Errors,
					Required:    true,
					Attrs: []Node{
						Class("input-amount"),
						Attr("inputmode", "decimal"),
						AutoComplete("off"),
						previewAttrs("input changed delay:300ms"),
					},
				}),
				CurrencySelect("Currency", "currency", d.Form.Currency, d.Errors, previewAttrs("change")),
			),
			components.Field(components.FieldProps{
				Label:    "Date",
				Name:     "date",
				Type:     "date",
				Value:    d.Form.Date,
				Errors:   d.Errors,
				Required: true,
			}),
			SplitSection(d),
			Div(Class("row"),
				components.SubmitButton("Save", false),
				A(Class("btn"), Href(d.LedgerURL()), Text("Cancel")),
			),
		),
	)
}

// SplitSection renders the split controls for the current mode.
func SplitSection(d ExpenseFormData) Node {
	d = withEvenPercentages(d)
	f := d.Form

	switchButton := func(to, text string) Node {
		return Button(Type("button"), Class("btn-link"),
			hx.Post(d.splitURL("section")),
			hx.Vals(fmt.Sprintf(`{"switch":%q}`, to)),
			hx.Target("#split-section"),
			hx.Swap("outerHTML"),
			Text(text),
		)
	}

	var content Node

	switch {
	case f.Mode == contract.ModeSimple && len(d.Participants) == 2:
		content = Group{
			Div(Class("label"), Text("How should it be split?")),
			SplitPreview(d),
			components.ErrorText(d.Errors.Get("preset")),
		}

	case f.Mode == contract.ModeSimple:
		content = Group{
			payerSelect(d, "Paid by", Text("and split equally among everyone")),
			SplitPreview(d),
		}

	default:
		content = Group{
			Input(Type("hidden"), Name("mode"), Value(contract.ModeAdvanced)),
			Div(Class("grid-2"),
				payerSelect(d, "Paid by", nil),
				components.SelectField(components.SelectProps{
					Label:   "Split",
					Name:    "split_type",
					Value:   string(f.SplitType),
					Options: splitTypeOptions(),
					Attrs: []Node{
						hx.Post(d.splitURL("values")),
						hx.Trigger("change"),
						hx.Target("#split-values"),
						hx.Swap("outerHTML"),
					},
				}),
			),
			SplitValues(d),
		}
	}

	return Div(ID("split-section"), Class("stack-sm"),
		If(f.Mode == contract.ModeSimple, Input(Type("hidden"), Name("mode"), Value(contract.ModeSimple))),
		content,
		Div(
			If(f.Mode == contract.ModeSimple, switchButton(contract.ModeAdvanced, "More options")),
			If(f.Mode != contract.ModeSimple, switchButton(contract.ModeSimple, "Back to simple split")),
		),
	)
}

// SplitValues renders the advanced split's value inputs and preview, the
// part that changes with the split type. The selects stay in place so that
// htmx doesn't refocus a swapped select, which reopens its picker on mobile.
func SplitValues(d ExpenseFormData) Node {
	d = withEvenPercentages(d)
	return Div(ID("split-values"), Class("stack-sm"),
		splitTable(d),
		SplitPreview(d),
	)
}

// withEvenPercentages starts an empty two-person percentage split at 50/50
// so that the slider, the inputs and the preview agree.
func withEvenPercentages(d ExpenseFormData) ExpenseFormData {
	f := d.Form
	if f.Mode == contract.ModeSimple || f.SplitType != split.Percentage || len(d.Participants) != 2 {
		return d
	}
	a, b := d.Participants[0].ID, d.Participants[1].ID
	if f.Values[a] != "" || f.Values[b] != "" {
		return d
	}
	nf := *f
	nf.Values = maps.Clone(f.Values)
	if nf.Values == nil {
		nf.Values = map[snowflake.ID]string{}
	}
	nf.Values[a], nf.Values[b] = "50", "50"
	d.Form = &nf
	return d
}

func payerSelect(d ExpenseFormData, label string, after Node) Node {
	var opts []components.SelectOption
	for _, p := range d.Participants {
		name := p.DisplayName()
		if p.ID == d.Me.ID {
			name = "You"
		}
		opts = append(opts, components.SelectOption{Value: p.ID.String(), Label: name})
	}

	return Div(Class("stack-sm"),
		components.SelectField(components.SelectProps{
			Label:   label,
			Name:    "paid_by",
			Value:   d.Form.PaidBy.String(),
			Options: opts,
			Errors:  d.Errors,
			Attrs: []Node{
				hx.Post(d.splitURL("preview")),
				hx.Trigger("change"),
				hx.Target("#split-preview"),
				hx.Swap("outerHTML"),
			},
		}),
		If(after != nil, P(Class("hint"), after)),
	)
}

func splitTypeOptions() []components.SelectOption {
	var opts []components.SelectOption
	for _, t := range split.AdvancedTypes {
		opts = append(opts, components.SelectOption{Value: string(t), Label: t.Label()})
	}
	return opts
}

func splitTable(d ExpenseFormData) Node {
	f := d.Form
	cur, _ := money.LookupCurrency(f.Currency)

	// With two people the second value follows from the first (see app.js).
	var pair string
	if len(d.Participants) == 2 && (f.SplitType == split.Percentage || f.SplitType == split.Exact) {
		pair = string(f.SplitType)
	}

	var unit, placeholder, hint string
	switch f.SplitType {
	case split.Percentage:
		unit, placeholder, hint = "%", "0", "Percentages must add up to 100%."
	case split.Shares:
		unit, placeholder, hint = "shares", "0", "Each person pays in proportion to their shares."
	case split.Exact:
		unit, placeholder, hint = cur.Code, "0.00", "Amounts must add up to the total."
	}

	names := make([]string, len(d.Participants))
	for i, p := range d.Participants {
		names[i] = p.DisplayName()
		if p.ID == d.Me.ID {
			names[i] = "You"
		}
	}

	previewOn := func(trigger string) Node {
		return Group{
			hx.Post(d.splitURL("preview")),
			hx.Trigger(trigger),
			hx.Target("#split-preview"),
			hx.Swap("outerHTML"),
		}
	}

	return Div(Class("stack-sm"),
		P(Class("hint"), Text(hint)),
		Table(Class("split-table"),
			If(pair != "", Data("pair", pair)),
			If(pair == string(split.Exact), Data("decimals", strconv.Itoa(int(cur.Decimals)))),
			TBody(
				Map(d.Participants, func(p *domain.User) Node {
					name := names[slices.Index(d.Participants, p)]
					key := contract.ValueKey(p.ID)

					return Tr(
						Td(Span(Text(name))),
						Td(Class("share"),
							Input(Type("text"), Name(key), Value(f.Values[p.ID]), Placeholder(placeholder),
								If(pair != "", Data("split-value", "")),
								Attr("inputmode", "decimal"), AutoComplete("off"),
								Aria("label", name+" "+unit),
								If(d.Errors.Get(key) != "", Class("invalid")),
								previewOn("input changed delay:300ms"),
							),
							components.ErrorText(d.Errors.Get(key)),
						),
						Td(Class("muted small nowrap"), Text(unit)),
					)
				}),
			),
		),
		If(pair == string(split.Percentage), percentSlider(names, f.Values[d.Participants[0].ID])),
	)
}

// percentSlider renders a slider for the first person's percentage of a
// two-person split. It isn't submitted: app.js copies it into the inputs.
func percentSlider(names []string, first string) Node {
	value := "50"
	if d, err := money.ParseDecimal(first); err == nil && d.LessThanOrEqual(decimal.NewFromInt(100)) {
		value = d.Round(0).String()
	}
	return Div(Class("split-slider stack-sm"),
		Input(Type("range"), Min("0"), Max("100"), Step("1"), Value(value), Data("split-slider", ""),
			Aria("label", names[0]+" percentage")),
		Div(Class("row-between muted small"),
			Span(Text(names[0])),
			Span(Text(names[1])),
		),
	)
}

// SplitPreview renders the live result of the split for the current form.
func SplitPreview(d ExpenseFormData) Node {
	f := d.Form
	me := d.Me.ID
	cur, amount, amountErrs := f.ParseAmount()
	amountOK := len(amountErrs) == 0

	people := People{}
	for _, p := range d.Participants {
		people[p.ID] = p
	}

	// Two-person simple mode: preset options with live sentences.
	if f.Mode == contract.ModeSimple && len(d.Participants) == 2 {
		var other snowflake.ID
		for _, p := range d.Participants {
			if p.ID != me {
				other = p.ID
			}
		}
		otherName := people.name(other)

		titles := map[split.Preset]string{
			split.YouPaidEqual:  "You paid, split equally",
			split.YouOwedFull:   "You are owed the full amount",
			split.TheyPaidEqual: otherName + " paid, split equally",
			split.TheyOwedFull:  otherName + " is owed the full amount",
		}

		return Div(ID("split-preview"), Class("options"),
			Map(split.Presets, func(p split.Preset) Node {
				sentence := "Enter an amount"
				tone := "muted"
				if amountOK {
					pf := *f
					pf.Preset = p
					if res, errs := pf.ResolveSplit(me, d.Participants, amount, cur); len(errs) == 0 {
						sentence = presetSentence(res, me, other, otherName, cur)
						tone = "neg"
						if res.PaidBy == me {
							tone = "pos"
						}
					}
				}

				return Label(Class("option"),
					Input(Type("radio"), Name("preset"), Value(string(p)), If(f.Preset == p, Checked())),
					Div(Class("grow"),
						Div(Class("title"), Text(titles[p])),
						Div(Class(tone+" small"), Text(sentence)),
					),
				)
			}),
		)
	}

	if !amountOK {
		return Div(ID("split-preview"), Class("hint"), Text("Enter an amount to see the split."))
	}

	res, errs := f.ResolveSplit(me, d.Participants, amount, cur)
	if len(errs) > 0 {
		msg := errs.Get("split")
		if msg == "" {
			msg = "Check the highlighted values."
			for _, v := range errs {
				if len(v) > 0 {
					msg = v[0]
					break
				}
			}
		}
		return Div(ID("split-preview"), P(Class("error-text"), Role("alert"), Text(msg)))
	}

	shares := res.Shares()

	return Div(ID("split-preview"), Class("preview stack-sm"),
		Map(d.Participants, func(p *domain.User) Node {
			share, ok := shares[p.ID]
			if !ok {
				return nil
			}
			return Div(Class("row-between small"),
				Span(Text(people.Name(p.ID, me))),
				Span(Class("nowrap"), Text(money.Format(share, cur))),
			)
		}),
	)
}

func presetSentence(res *contract.ResolvedSplit, me, other snowflake.ID, otherName string, cur money.Currency) string {
	shares := res.Shares()
	if res.PaidBy == me {
		return fmt.Sprintf("%s owes you %s", otherName, money.Format(shares[other], cur))
	}
	return fmt.Sprintf("You owe %s %s", otherName, money.Format(shares[me], cur))
}

// splitLabel describes how an expense is split. An equal split with a single
// participant is the "owed the full amount" preset, not an equal split.
func splitLabel(e *domain.Expense) string {
	if e.SplitType == split.Equal && len(e.Participants) == 1 {
		return "Full amount"
	}
	return e.SplitType.Label()
}

// ExpenseDetailMain renders an expense.
func ExpenseDetailMain(d GroupData, e *domain.Expense) Node {
	base := groupURL(d.Group.ID)
	me := d.Me.ID
	self := base + "/expenses/" + e.ID.String()

	title := e.Description
	if e.Kind == domain.KindSettlement {
		title = "Payment"
	}

	return Div(Class("stack"),
		Div(Class("page-head"),
			Div(Class("grow"),
				P(Class("sub"), ledgerLink(d)),
				H1(Text(title)),
			),
		),
		Div(Class("card stack"),
			Div(Class("row-between"),
				Div(
					Div(Class("big-amount"), Text(Fmt(e.Amount, e.Currency))),
					Div(Class("muted small"), Text(e.Date.Format("Monday, 2 January 2006"))),
				),
				If(e.Kind == domain.KindExpense, Span(Class("muted small"), Text(splitLabel(e)))),
			),
			Ul(Class("list"),
				Li(Div(Class("list-item flush"),
					Avatar(d.People.name(e.PaidBy)),
					Div(Class("grow"),
						Text(d.People.Name(e.PaidBy, me)), Text(" paid "), Strong(Text(Fmt(e.Amount, e.Currency))),
					),
				)),
				Map(e.Participants, func(p domain.Participant) Node {
					return Li(Div(Class("list-item flush"),
						Avatar(d.People.name(p.UserID)),
						Div(Class("grow"), Text(participantLine(e, p, d.People, me))),
					))
				}),
			),
			P(Class("muted small"),
				Textf("Added by %s", lower(d.People.Name(e.AddedBy, me))),
				If(!e.UpdatedAt.IsZero(), Textf(" · updated %s", e.UpdatedAt.In(d.Me.Location()).Format("2 Jan 2006"))),
			),
		),
		Div(Class("row"),
			If(e.Kind == domain.KindExpense, A(Class("btn"), Href(self+"/edit"), Text("Edit"))),
			Button(Type("button"), Class("btn btn-danger"),
				hx.Post(self+"/delete"), hx.Confirm("Delete this "+strings.ToLower(title)+"?"),
				Text("Delete"),
			),
		),
	)
}

func participantLine(e *domain.Expense, p domain.Participant, people People, me snowflake.ID) string {
	name := people.Name(p.UserID, me)
	amount := Fmt(p.Share, e.Currency)

	switch {
	case e.Kind == domain.KindSettlement:
		return fmt.Sprintf("%s received %s", name, amount)
	case p.UserID == e.PaidBy && p.UserID == me:
		return fmt.Sprintf("Your share is %s", amount)
	case p.UserID == e.PaidBy:
		return fmt.Sprintf("%s's share is %s", name, amount)
	case p.UserID == me:
		return fmt.Sprintf("You owe %s", amount)
	default:
		return fmt.Sprintf("%s owes %s", name, amount)
	}
}
