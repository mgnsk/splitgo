package splitgo

import (
	"embed"
	"io/fs"

	"github.com/mgnsk/webkit/assets"
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

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}
