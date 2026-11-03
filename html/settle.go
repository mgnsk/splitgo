package html

import (
	"net/url"

	"github.com/mgnsk/splitgo/contract"
	"github.com/mgnsk/splitgo/html/components"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
)

// SettleMain renders the record payment page.
func SettleMain(d GroupData, form contract.SettleForm, errs url.Values) Node {
	base := groupURL(d.Group.ID)

	var opts []components.SelectOption
	for _, m := range d.Members {
		opts = append(opts, components.SelectOption{Value: m.ID.String(), Label: d.People.Name(m.ID, d.Me.ID)})
	}

	return Div(Class("stack"),
		Div(Class("page-head"),
			Div(
				H1(Text("Record a payment")),
				P(Class("sub"), A(Href(base), Text(d.Group.Name))),
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
				A(Class("btn"), Href(base), Text("Cancel")),
			),
		),
	)
}
