//go:build !production

package inspector

import (
	"io/fs"
	"os"
)

func Assets() (fs.FS, error) {
	return os.DirFS("web/dist"), nil
}
