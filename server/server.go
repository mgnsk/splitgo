// Package server glues the webkit framework to the splitgo views and models.
package server

import (
	"net/http"

	"github.com/mgnsk/splitgo/html"
	"github.com/mgnsk/webkit/web"
	"maragu.dev/gomponents"
)

// NewHandler builds the application's HTTP handler. Each registrar is called
// with the mux to register its routes, then the result is wrapped with the
// global middleware.
func NewHandler(registrars ...func(*http.ServeMux)) http.Handler {
	return web.NewHandler(web.Options{
		Errors: web.ErrorRenderer{
			Page: func(r *http.Request, code int, msg string) gomponents.Node {
				return html.Page(html.PageProps{
					Title:    "Error",
					Path:     r.URL.Path,
					Children: html.ErrorMain(code, msg),
				})
			},
			Toast: func(msg string) gomponents.Node {
				return html.Toast(false, msg)
			},
			ToastTarget: "#toast",
		},
	}, registrars...)
}
