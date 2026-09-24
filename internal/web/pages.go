package web

import (
	"bytes"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"github.com/dev-labs-ai/weather-panel/internal/view"
)

// pages renders the HTML routes.
type pages struct {
	logger *slog.Logger
}

// index renders the panel with an empty result region.
func (p *pages) index(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, http.StatusOK, view.Page("", nil))
}

// render writes c as an HTML response with the status. It renders into a buffer first, so a failing component
// becomes a 500 instead of a truncated page.
func (p *pages) render(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	var buf bytes.Buffer
	if err := c.Render(r.Context(), &buf); err != nil {
		p.logger.LogAttrs(r.Context(), slog.LevelError, "render failed", slog.String("error", err.Error()))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}
