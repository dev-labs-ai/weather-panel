package web

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/a-h/templ"

	"github.com/dev-labs-ai/weather-panel/internal/view"
	"github.com/dev-labs-ai/weather-panel/internal/weather"
)

// Lookuper runs a weather lookup; *weather.Service is the implementation.
type Lookuper interface {
	Lookup(ctx context.Context, input string) (weather.Report, error)
}

// pages renders the HTML routes.
type pages struct {
	logger *slog.Logger
	lookup Lookuper
}

// index renders the panel with an empty result region.
func (p *pages) index(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, http.StatusOK, view.Page("", nil))
}

// weather looks up the city in the query string. An htmx request gets the result region's content plus the text
// for the screen reader announcer, which htmx swaps in out of band; any other request gets the whole page, with the
// typed city kept in the input, so the search works without JavaScript. The status code reflects the outcome, and
// htmx 4 swaps error responses into the result region like any other, so a previous result never stays next to a
// new error.
func (p *pages) weather(w http.ResponseWriter, r *http.Request) {
	city := r.URL.Query().Get("city")
	report, err := p.lookup.Lookup(r.Context(), city)
	status, content, announcement := outcome(report, err)

	w.Header().Add("Vary", "HX-Request")
	if r.Header.Get("HX-Request") == "true" {
		p.render(w, r, status, templ.Join(content, view.Announcement(announcement)))
		return
	}
	p.render(w, r, status, view.Page(city, content))
}

// outcome maps a lookup's result to the response status, the result region's content, and the text announced to
// screen readers.
func outcome(report weather.Report, err error) (int, templ.Component, string) {
	var verr weather.ValidationError
	switch {
	case err == nil:
		return http.StatusOK, view.Result(report), view.ResultAnnouncement(report)
	case errors.As(err, &verr):
		return http.StatusUnprocessableEntity, view.ValidationMessage(verr.Message),
			view.ValidationAnnouncement(verr.Message)
	case errors.Is(err, weather.ErrNotFound):
		return http.StatusNotFound, view.NotFoundMessage(), view.NotFoundAnnouncement()
	case errors.Is(err, weather.ErrUpstreamTimeout):
		return http.StatusGatewayTimeout, view.UnavailableMessage(), view.UnavailableAnnouncement()
	default:
		return http.StatusBadGateway, view.UnavailableMessage(), view.UnavailableAnnouncement()
	}
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
