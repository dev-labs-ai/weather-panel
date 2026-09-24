package web

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// NewRouter returns the service's HTTP handler with every route and middleware in place.
func NewRouter(logger *slog.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(securityHeaders, requestLogger(logger), recoverer(logger))

	r.Get("/healthz", healthz)
	return r
}

// healthz answers the liveness probe; it succeeds while the process is serving requests.
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}
