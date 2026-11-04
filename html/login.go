package html

import (
	"net/url"

	"github.com/mgnsk/splitgo/html/components"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
)

// LoginMain renders the login page.
func LoginMain(email, next string, errs url.Values) Node {
	return Div(Class("stack"),
		Div(Class("stack-sm"),
			H1(Text("Sign in")),
			P(Class("muted"), Text("We'll email you a link to sign in. No password needed.")),
		),
		Form(Class("card stack"), Method("post"), Action("/login"),
			Input(Type("hidden"), Name("next"), Value(next)),
			components.Field(components.FieldProps{
				Label:       "Email",
				Name:        "email",
				Type:        "email",
				Value:       email,
				Placeholder: "you@example.com",
				Errors:      errs,
				Required:    true,
				Attrs:       []Node{AutoComplete("email"), AutoFocus()},
			}),
			components.SubmitButton("Email me a sign-in link", true),
		),
	)
}

// CheckEmailMain renders the page shown after requesting a sign-in link, with
// a form to enter the emailed code instead.
func CheckEmailMain(email, next string, errs url.Values) Node {
	return Div(Class("stack"),
		Div(Class("stack-sm"),
			H1(Text("Check your email")),
			P(Text("If "), Strong(Text(email)), Text(" can sign in, a code and a sign-in link are on their way. They expire in 15 minutes.")),
		),
		Form(Class("card stack"), Method("post"), Action("/login/code"),
			Input(Type("hidden"), Name("email"), Value(email)),
			Input(Type("hidden"), Name("next"), Value(next)),
			components.Field(components.FieldProps{
				Label:       "Sign-in code",
				Name:        "code",
				Placeholder: "123456",
				Hint:        "Enter the code here, or open the link in the email.",
				Errors:      errs,
				Required:    true,
				Attrs: []Node{
					Class("input-code"),
					Attr("inputmode", "numeric"),
					AutoComplete("one-time-code"),
					MaxLength("7"),
					AutoFocus(),
				},
			}),
			components.ErrorText(errs.Get("email")),
			components.SubmitButton("Sign in", true),
		),
		P(Class("muted small"), A(Href("/login"), Text("Use a different email"))),
	)
}

// ConfirmLoginMain renders the magic link confirmation page.
func ConfirmLoginMain(action, email string) Node {
	return Form(Class("card stack"), Method("post"), Action(action),
		Div(Class("stack-sm"),
			H1(Text("Sign in")),
			P(Text("Continue as "), Strong(Text(email)), Text("?")),
		),
		components.SubmitButton("Continue", true),
	)
}

// InvalidLinkMain renders the page for an invalid or expired sign-in link.
func InvalidLinkMain() Node {
	return Div(Class("card stack-sm"),
		H1(Text("Link expired")),
		P(Class("muted"), Text("This sign-in link is invalid, expired or has already been used.")),
		P(A(Class("btn btn-primary"), Href("/login"), Text("Get a new link"))),
	)
}
