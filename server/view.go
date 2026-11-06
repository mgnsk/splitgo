package server

import (
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/mgnsk/splitgo/html"
	"github.com/mgnsk/webkit/session"
	"github.com/mgnsk/webkit/web"
	"maragu.dev/gomponents"
)

// RenderPage renders a full HTML page with the layout.
func RenderPage(
	w http.ResponseWriter,
	r *http.Request,
	sm *scs.SessionManager,
	title string,
	content gomponents.Node,
) {
	// Note: Pop must be before writing headers.
	successMessage := session.PopFlash(r.Context(), sm)

	web.Render(w, html.Page(html.PageProps{
		Title:        title,
		User:         GetUser(r.Context()),
		Path:         r.URL.Path,
		Children:     content,
		FlashSuccess: successMessage,
	}))
}

// RenderPartial renders an HTML fragment.
func RenderPartial(w http.ResponseWriter, node gomponents.Node) {
	web.Render(w, node)
}

// Redirect redirects the client. For htmx requests it sets the HX-Redirect
// header so that htmx performs a full navigation.
func Redirect(w http.ResponseWriter, r *http.Request, location string) {
	web.Redirect(w, r, location)
}

// Flash sets a success flash message shown on the next page.
func Flash(r *http.Request, sm *scs.SessionManager, msg string) {
	session.Flash(r.Context(), sm, msg)
}
