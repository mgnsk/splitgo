package html

import (
	"net/url"
	"slices"

	"github.com/mgnsk/splitgo/contract"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/html/components"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
)

// SettleMain renders the record payment page.
// parties are the people who can pay or receive: the members and former
// members with open debts.
func SettleMain(d GroupData, parties []*domain.User, form contract.SettleForm, errs url.Values) Node {
	base := groupURL(d.Group.ID)

	var opts []components.SelectOption
	for _, p := range parties {
		label := d.People.Name(p.ID, d.Me.ID)
		if !slices.ContainsFunc(d.Members, func(m *domain.User) bool { return m.ID == p.ID }) {
			label += " (left the group)"
		}
		opts = append(opts, components.SelectOption{Value: p.ID.String(), Label: label})
	}

	return Div(Class("stack"),
		Div(Class("page-head"),
			Div(
				H1(Text("Record a payment")),
				P(Class("sub"), ledgerLink(d)),
			),
		),
		Form(Class("card stack"), Method("post"), Action(base+"/settle"),
			Div(Class("grid-2"),
				components.SelectField(components.SelectProps{
					Label: "From", Name: "from", Value: form.From.String(), Options: opts, Errors: errs,
				}),
				components.SelectField(components.SelectProps{
					Label: "To", Name: "to", Value: form.To.String(), Options: opts, Errors: errs,
				}),
			),
			Div(Class("grid-amount"),
				components.Field(components.FieldProps{
					Label:    "Amount",
					Name:     "amount",
					Value:    form.Amount,
					Errors:   errs,
					Required: true,
					Attrs:    []Node{Class("input-amount"), Attr("inputmode", "decimal"), AutoComplete("off")},
				}),
				CurrencySelect("Currency", "currency", form.Currency, errs),
			),
			components.Field(components.FieldProps{
				Label: "Date", Name: "date", Type: "date", Value: form.Date, Errors: errs, Required: true,
			}),
			P(Class("hint"), Text("Record money paid outside splitgo, e.g. a bank transfer or cash.")),
			Div(Class("row"),
				components.SubmitButton("Record payment", false),
				A(Class("btn"), Href(d.LedgerURL()), Text("Cancel")),
			),
		),
	)
}
