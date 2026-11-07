package html

import (
	"net/url"

	"github.com/mgnsk/splitgo/contract"
	"github.com/mgnsk/splitgo/domain"
	"github.com/mgnsk/splitgo/html/components"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
)

// AccountMain renders the account page.
func AccountMain(me *domain.User, form contract.AccountForm, errs url.Values) Node {
	return Div(Class("stack"),
		H1(Text("Account")),
		Form(Class("card stack"), Method("post"), Action("/account"),
			Div(Class("field"),
				Span(Class("label"), Text("Email")),
				P(Class("muted"), Text(me.Email)),
			),
			components.Field(components.FieldProps{
				Label:       "Name",
				Name:        "name",
				Value:       form.Name,
				Placeholder: me.DisplayName(),
				Hint:        "Shown to people in your groups.",
				Errors:      errs,
			}),
			CurrencySelect("Default currency for new groups", "currency", form.Currency, errs),
			components.SelectField(components.SelectProps{
				Label:   "Appearance",
				Name:    "theme",
				Value:   form.Theme,
				Options: themeSelectOptions(),
				Errors:  errs,
			}),
			Div(components.SubmitButton("Save", false)),
		),
		notificationsCard(),
		Form(Method("post"), Action("/logout"),
			Button(Type("submit"), Class("btn"), Text("Sign out")),
		),
	)
}

func themeSelectOptions() []components.SelectOption {
	opts := make([]components.SelectOption, len(ThemeOptions))
	for i, t := range ThemeOptions {
		opts[i] = components.SelectOption{Value: string(t.Value), Label: t.Label}
	}
	return opts
}

// notificationsCard renders the push notification switch for this device.
// app.js fills in the status and shows the button when push is available.
func notificationsCard() Node {
	return Div(Class("card stack-sm"), ID("push"),
		H2(Text("Notifications")),
		P(Class("muted"), Text("Get notified on this device when someone adds an expense or payment involving you, or adds you to a group.")),
		P(Class("muted"), Data("push-status", ""),
			Text("Notifications are not supported in this browser. On iPhone and iPad, tap Share, then Add to Home Screen, and open splitgo from the Home Screen."),
		),
		Div(Button(Type("button"), Class("btn"), Data("push-toggle", ""), Hidden(""))),
	)
}
