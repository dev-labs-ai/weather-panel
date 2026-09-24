// Package web embeds the static assets that the service serves under /static/.
package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"sync"
)

//go:embed static
var embedded embed.FS

// Static holds the files under web/static, named relative to it, such as "js/htmx.min.js". css/app.css is present
// only when `make css` ran before the build.
var Static = mustSub(embedded, "static")

var assetVersions = sync.OnceValue(func() map[string]string { return versions(Static) })

// AssetPath returns the URL of a static file with a version derived from its content, such as
// "/static/js/htmx.min.js?v=0123456789ab". Assets are served with a long-lived cache lifetime, and the version makes a
// browser fetch a file again after it changes. A missing file gets no version.
func AssetPath(name string) string {
	path := "/static/" + name
	if v, ok := assetVersions()[name]; ok {
		return path + "?v=" + v
	}
	return path
}

// versions hashes every file in fsys.
func versions(fsys fs.FS) map[string]string {
	out := map[string]string{}
	_ = fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[name] = hex.EncodeToString(sum[:6])
		return nil
	})
	return out
}

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
