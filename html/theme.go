package html

import (
	"github.com/mgnsk/splitgo/domain"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/html"
)

const (
	moonIcon = `<svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/></svg>`
	sunIcon  = `<svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>`
)

// themeAttr returns the data-theme attribute for an explicit theme choice.
// With the system theme no attribute is set and CSS follows the OS setting.
func themeAttr(user *domain.User) Node {
	if user == nil || (user.Theme != domain.ThemeLight && user.Theme != domain.ThemeDark) {
		return nil
	}
	return Data("theme", string(user.Theme))
}

// themeToggle renders the light/dark toggle. Both buttons are rendered and
// CSS shows the one switching away from the currently effective theme, so it
// also works when following the OS setting.
func themeToggle(path string) Node {
	return Form(Class("theme-toggle"), Method("post"), Action("/theme"),
		Input(Type("hidden"), Name("next"), Value(path)),
		Button(Type("submit"), Class("icon-btn to-dark"), Name("theme"), Value(string(domain.ThemeDark)),
			Title("Switch to dark mode"), Aria("label", "Switch to dark mode"), Raw(moonIcon)),
		Button(Type("submit"), Class("icon-btn to-light"), Name("theme"), Value(string(domain.ThemeLight)),
			Title("Switch to light mode"), Aria("label", "Switch to light mode"), Raw(sunIcon)),
	)
}

// ThemeOptions are the theme choices for the account page.
var ThemeOptions = []struct {
	Value domain.Theme
	Label string
}{
	{domain.ThemeSystem, "Same as system"},
	{domain.ThemeLight, "Light"},
	{domain.ThemeDark, "Dark"},
}

// Browser UI colors matching the top bar in each theme.
const (
	themeColorLight = "#ffffff"
	themeColorDark  = "#171a20"
)

// themeColor renders the theme-color meta tags for the browser and
// installed app UI, following the user's theme or the OS setting.
func themeColor(user *domain.User) Node {
	if user != nil {
		switch user.Theme {
		case domain.ThemeLight:
			return Meta(Name("theme-color"), Content(themeColorLight))
		case domain.ThemeDark:
			return Meta(Name("theme-color"), Content(themeColorDark))
		}
	}
	return Group{
		Meta(Name("theme-color"), Content(themeColorLight), Attr("media", "(prefers-color-scheme: light)")),
		Meta(Name("theme-color"), Content(themeColorDark), Attr("media", "(prefers-color-scheme: dark)")),
	}
}
