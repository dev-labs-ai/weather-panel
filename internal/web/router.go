package web

import (
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter returns the service's HTTP handler with every route and middleware in place. static holds the files
// served under /static/, and lookup answers the searches.
func NewRouter(logger *slog.Logger, static fs.FS, lookup Lookuper) http.Handler {
	r := chi.NewRouter()
	r.Use(securityHeaders, requestLogger(logger), recoverer(logger), middleware.GetHead)

	p := &pages{logger: logger, lookup: lookup}
	r.Get("/", p.index)
	r.Get("/weather", p.weather)
	r.Get("/healthz", healthz)
	r.Get("/static/*", staticFiles(static))
	return r
}

// healthz answers the liveness probe; it succeeds while the process is serving requests.
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

// staticFiles serves the files in fsys with a one-year cache lifetime. Pages reference them through
// web.AssetPath, whose content version changes the URL whenever a file changes. Directories are not listed, and a
// missing file gets a plain 404 that browsers do not keep.
func staticFiles(fsys fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "*")
		if info, err := fs.Stat(fsys, name); err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeFileFS(w, r, fsys, name)
	}
}
