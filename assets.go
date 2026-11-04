package splitgo

import (
	"embed"
	"fmt"
	"hash/crc32"
	"io/fs"
	"mime"
	"net/http"
	"sync"
	"time"
)

//go:embed assets
var embeddedAssets embed.FS

var assetsFS = must(fs.Sub(embeddedAssets, "assets"))

var assetPaths sync.Map

// GetAssetPath returns the asset path with appended
// CRC checksum query parameter for cache-busting.
func GetAssetPath(name string) string {
	if p, ok := assetPaths.Load(name); ok {
		return p.(string)
	}

	b, err := fs.ReadFile(assetsFS, name)
	if err != nil {
		panic(err)
	}

	p := fmt.Sprintf("/assets/%s?crc=%d", name, crc32.ChecksumIEEE(b))
	assetPaths.Store(name, p)

	return p
}

// RegisterAssetsHandler registers the static assets HTTP handler.
func RegisterAssetsHandler(mux *http.ServeMux) {
	handler := newAssetCacheMiddleware(30 * 24 * time.Hour)(
		http.StripPrefix("/assets", http.FileServerFS(assetsFS)),
	)

	mux.Handle("GET /assets/", handler)

	// The service worker must be served from the root to control the whole
	// app, and must not be cached long so that updates are picked up.
	mux.HandleFunc("GET /sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		http.ServeFileFS(w, r, assetsFS, "sw.js")
	})
}

func init() {
	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
		panic(err)
	}
}

func newAssetCacheMiddleware(d time.Duration) func(http.Handler) http.Handler {
	value := fmt.Sprintf("max-age=%d, immutable", int64(d.Seconds()))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", value)

			next.ServeHTTP(w, r)
		})
	}
}

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}
