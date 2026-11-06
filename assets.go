package splitgo

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/mgnsk/webkit/assets"
)

//go:embed assets
var embeddedAssets embed.FS

var assetsFS = must(fs.Sub(embeddedAssets, "assets"))

// Assets are the static assets.
var Assets = assets.New(assetsFS)

// GetAssetPath returns the asset path with appended
// CRC checksum query parameter for cache-busting.
func GetAssetPath(name string) string {
	return Assets.Path(name)
}

// RegisterAssetsHandler registers the static assets HTTP handler.
func RegisterAssetsHandler(mux *http.ServeMux) {
	Assets.Register(mux)

	// The service worker must be served from the root to control the whole
	// app, and must not be cached long so that updates are picked up.
	mux.HandleFunc("GET /sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		http.ServeFileFS(w, r, assetsFS, "sw.js")
	})
}

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}
