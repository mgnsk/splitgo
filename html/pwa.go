package html

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"net/http"
	"slices"
	"strings"

	"github.com/mgnsk/splitgo"
	"github.com/mgnsk/webkit/pwa"
	. "maragu.dev/gomponents"
	. "maragu.dev/gomponents/components"
	. "maragu.dev/gomponents/html"
)

// PWA serves the web app manifest, service worker and client script.
var PWA = pwa.New(pwa.Options{
	Manifest: pwa.Manifest{
		Name:            "splitgo",
		ShortName:       "splitgo",
		Description:     "Share expenses with friends",
		BackgroundColor: "#f6f7f9",
		ThemeColor:      "#ffffff",
		Icons: []pwa.Icon{
			{Src: splitgo.Assets.Path("icons/icon-192.png"), Sizes: "192x192", Type: "image/png", Purpose: "any"},
			{Src: splitgo.Assets.Path("icons/icon-512.png"), Sizes: "512x512", Type: "image/png", Purpose: "any"},
			{Src: splitgo.Assets.Path("icons/icon-maskable-512.png"), Sizes: "512x512", Type: "image/png", Purpose: "maskable"},
		},
	},
	// Both URLs carry a checksum, so a change to either replaces the
	// service worker's cached copies.
	OfflinePage: OfflinePath,
	Precache:    []string{splitgo.Assets.Path("app.css")},
})

// stylesheets and scripts are the paths of the stylesheets and scripts that
// pages load, in order.
var (
	stylesheets = []string{splitgo.Assets.Path("app.css")}
	scripts     = []string{
		splitgo.Assets.Path("htmx.min.js"),
		PWA.ScriptPath(),
		splitgo.Assets.Path("app.js"),
	}
)

// Version identifies the stylesheets and scripts that pages load: a
// checksum of their paths, which carry checksums of their contents. It
// changes on a deploy that changes any of them.
var Version = fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(strings.Join(slices.Concat(stylesheets, scripts), "\n"))))

// offlinePage is the page the service worker shows for navigations while
// offline.
var offlinePage = renderOfflinePage()

// OfflinePath is the URL path of the offline page, with a checksum query
// parameter so that the service worker caches each version anew.
var OfflinePath = fmt.Sprintf("/offline?crc=%d", crc32.ChecksumIEEE(offlinePage))

func renderOfflinePage() []byte {
	var b bytes.Buffer
	if err := HTML5(HTML5Props{
		Title:    "Offline · splitgo",
		Language: "en",
		Head: []Node{
			Meta(Name("viewport"), Content("width=device-width, initial-scale=1")),
			Link(Rel("stylesheet"), Href(splitgo.Assets.Path("app.css"))),
		},
		Body: []Node{
			topbar(nil, ""),
			Main(Class("container"),
				Div(Class("card stack-sm"),
					H1(Text("You're offline")),
					P(Class("muted"), Text("splitgo needs a connection to show up-to-date balances.")),
					P(A(Class("btn btn-primary"), Href("/"), Text("Try again"))),
				),
			),
		},
	}).Render(&b); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// RegisterOffline registers the offline page. Its URL changes with its
// content, so it is cached as immutable.
func RegisterOffline(mux *http.ServeMux) {
	mux.HandleFunc("GET /offline", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "max-age=2592000, immutable")
		_, _ = w.Write(offlinePage)
	})
}
