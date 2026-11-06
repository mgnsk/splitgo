package splitgo

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/mgnsk/webkit/assets"
)

//go:embed assets
var embeddedAssets embed.FS

// Assets are the static assets. The service worker is also served from the
// root so that it controls the whole app.
var Assets = assets.New(must(fs.Sub(embeddedAssets, "assets")), assets.WithRootFile("sw.js"))

// GetAssetPath returns the asset path with appended
// CRC checksum query parameter for cache-busting.
func GetAssetPath(name string) string {
	return Assets.Path(name)
}

// RegisterAssetsHandler registers the static assets HTTP handler.
func RegisterAssetsHandler(mux *http.ServeMux) {
	Assets.Register(mux)
}

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}
