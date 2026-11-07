package splitgo

import (
	"embed"
	"io/fs"

	"github.com/mgnsk/webkit/assets"
	"github.com/mgnsk/webkit/pwa"
)

//go:embed assets
var embeddedAssets embed.FS

// Migrations are the SQL migrations, in the migrations directory.
//
//go:embed migrations/*.sql
var Migrations embed.FS

var assetsFS = must(fs.Sub(embeddedAssets, "assets"))

// Assets are the static assets.
var Assets = assets.New(assetsFS)

// PWA serves the web app manifest, service worker and client script.
var PWA = pwa.New(pwa.Options{
	Manifest: pwa.Manifest{
		Name:            "splitgo",
		ShortName:       "splitgo",
		Description:     "Share expenses with friends",
		BackgroundColor: "#f6f7f9",
		ThemeColor:      "#ffffff",
		Icons: []pwa.Icon{
			{Src: Assets.Path("icons/icon-192.png"), Sizes: "192x192", Type: "image/png", Purpose: "any"},
			{Src: Assets.Path("icons/icon-512.png"), Sizes: "512x512", Type: "image/png", Purpose: "any"},
			{Src: Assets.Path("icons/icon-maskable-512.png"), Sizes: "512x512", Type: "image/png", Purpose: "maskable"},
		},
	},
	// offline.html links the stylesheet without a checksum.
	OfflinePage: "/assets/offline.html",
	Precache:    []string{"/assets/app.css"},
})

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}
