// Package html implements the HTML views.
package html

import (
	"fmt"
	"strings"

	"github.com/mgnsk/splitgo"
	"github.com/mgnsk/splitgo/domain"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/components"
	. "maragu.dev/gomponents/html"
)

// htmxConfig makes htmx swap error responses so that error toasts are shown.
const htmxConfig = `{"responseHandling":[{"code":"204","swap":false},{"code":"[23]..","swap":true},{"code":"[45]..","swap":true,"error":true}],"includeIndicatorStyles":false}`

// PageProps is props for page.
type PageProps struct {
	Title        string
	User         *domain.User
	Path         string
	Children     Node
	FlashSuccess string
}

// Page renders a page.
func Page(props PageProps) Node {
	title := "splitgo"
	if props.Title != "" {
		title = props.Title + " · splitgo"
	}

	return HTML5(HTML5Props{
		Title:     title,
		Language:  "en",
		HTMLAttrs: Group{themeAttr(props.User), timezoneAttr(props.User)},
		Head: []Node{
			Meta(Name("viewport"), Content("width=device-width, initial-scale=1")),
			Meta(
				Attr("http-equiv", "Content-Security-Policy"),
				Attr("content", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:"),
			),
			Meta(Name("htmx-config"), Content(htmxConfig)),
			Link(Rel("icon"), Type("image/svg+xml"), Href(splitgo.Assets.Path("favicon.svg"))),
			Link(Rel("manifest"), Href(splitgo.PWA.ManifestPath())),
			Link(Rel("apple-touch-icon"), Href(splitgo.Assets.Path("icons/apple-touch-icon.png"))),
			Meta(Name("mobile-web-app-capable"), Content("yes")),
			Meta(Name("apple-mobile-web-app-capable"), Content("yes")),
			Meta(Name("apple-mobile-web-app-title"), Content("splitgo")),
			Meta(Name("apple-mobile-web-app-status-bar-style"), Content("default")),
			themeColor(props.User),
			Link(Rel("stylesheet"), Href(splitgo.Assets.Path("app.css"))),
			Script(Defer(), Src(splitgo.Assets.Path("htmx.min.js"))),
			Script(Defer(), Src(splitgo.PWA.ScriptPath())),
			Script(Defer(), Src(splitgo.Assets.Path("app.js"))),
		},
		Body: []Node{
			topbar(props.User, props.Path),
			Main(Class("container"), props.Children),
			If(props.User != nil && props.Path != "/account", pushPrompt()),
			Div(ID("toast"), Attr("aria-live", "polite"),
				If(props.FlashSuccess != "", Toast(true, props.FlashSuccess)),
			),
		},
	})
}

// timezoneAttr renders the user's saved time zone so that app.js can
// update it when the browser's differs.
func timezoneAttr(user *domain.User) Node {
	if user == nil {
		return nil
	}
	return Data("timezone", user.Timezone)
}

func topbar(user *domain.User, path string) Node {
	navLink := func(href, text string, active bool) Node {
		return A(Href(href), Text(text), Classes{"active": active})
	}

	return Header(Class("topbar"),
		Div(Class("topbar-inner"),
			A(Class("brand"), Href("/"), Text("splitgo")),
			If(user != nil, Group{
				Nav(Class("nav"),
					navLink("/", "Home", path == "/"),
					navLink("/activity", "Activity", path == "/activity"),
					navLink("/account", "Account", path == "/account"),
				),
				themeToggle(path),
			}),
		),
	)
}

// Toast renders a toast message.
func Toast(success bool, message string) Node {
	return Div(Classes{"toast": true, "toast-error": !success}, Role("status"), Text(message))
}

// ErrorMain renders an error page.
func ErrorMain(code int, msg string) Node {
	return Div(Class("stack"),
		Div(Class("card stack-sm"),
			H1(Textf("Error %d", code)),
			P(Class("muted"), Text(msg)),
			P(A(Href("/"), Text("Back to home"))),
		),
	)
}

// initials returns up to two initials of a display name.
func initials(name string) string {
	fields := strings.FieldsFunc(name, func(r rune) bool {
		return r == ' ' || r == '.' || r == '_' || r == '-'
	})

	var b strings.Builder
	for _, f := range fields {
		for _, r := range f {
			b.WriteRune(r)
			break
		}
		if b.Len() >= 2 {
			break
		}
	}

	if b.Len() == 0 {
		return "?"
	}

	return b.String()
}

// Avatar renders a user avatar with initials.
func Avatar(name string) Node {
	return Span(Class("avatar"), Aria("hidden", "true"), Text(initials(name)))
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}

// pushPrompt renders the notification offer that app.js shows once in the
// installed app.
func pushPrompt() Node {
	return Div(ID("push-prompt"), Class("push-prompt stack-sm"), Hidden(""),
		Role("dialog"), Aria("labelledby", "push-prompt-title"),
		P(ID("push-prompt-title"), Class("push-prompt-title"), Text("Turn on notifications?")),
		P(Text("Get notified when someone adds an expense or payment involving you, or adds you to a group.")),
		P(Class("muted small"), Text("You can change this later in Account settings.")),
		P(Class("small error-text"), Data("push-prompt-status", ""), Aria("live", "polite")),
		Div(Class("row"),
			Button(Type("button"), Class("btn btn-primary"), Data("push-prompt-enable", ""), Text("Turn on")),
			Button(Type("button"), Class("btn"), Data("push-prompt-dismiss", ""), Text("Not now")),
		),
	)
}
