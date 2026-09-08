//go:build production

package inspector

import (
	"embed"
	"io/fs"
)

//go:embed web/dist
var assets embed.FS

func Assets() (fs.FS, error) {
	return fs.Sub(assets, "web/dist")
}
