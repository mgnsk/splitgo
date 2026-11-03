package server

import (
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/mgnsk/splitgo/html"
	"maragu.dev/gomponents"
	hxhttp "maragu.dev/gomponents-htmx/http"
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
	successMessage := sm.PopString(r.Context(), SessionKeyFlashSuccess)

	w.Header().Set("Content-Type", "text/html; charset=UTF-8")

	if err := html.Page(html.PageProps{
		Title:        title,
		User:         GetUser(r.Context()),
		Path:         r.URL.Path,
		Children:     content,
		FlashSuccess: successMessage,
	}).Render(w); err != nil {
		panic(err)
	}
}

// RenderPartial renders an HTML fragment.
func RenderPartial(w http.ResponseWriter, node gomponents.Node) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	if err := node.Render(w); err != nil {
		panic(err)
	}
}

// Redirect redirects the client. For htmx requests it sets the HX-Redirect
// header so that htmx performs a full navigation.
func Redirect(w http.ResponseWriter, r *http.Request, location string) {
	if hxhttp.IsRequest(r.Header) {
		w.Header().Set("HX-Redirect", location)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}

// Flash sets a success flash message shown on the next page.
func Flash(r *http.Request, sm *scs.SessionManager, msg string) {
	sm.Put(r.Context(), SessionKeyFlashSuccess, msg)
}
