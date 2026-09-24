package web

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecovererAnswers500WithoutLoggingClientAddress(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	handler := recoverer(slog.New(slog.NewJSONHandler(&logs, nil)))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	req := httptest.NewRequest(http.MethodGet, "/weather?city=Recife", nil)
	req.RemoteAddr = "203.0.113.7:51234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	line := logs.String()
	if !strings.Contains(line, "boom") {
		t.Errorf("log %q does not record the panic", line)
	}
	for _, leak := range []string{"Recife", "203.0.113.7"} {
		if strings.Contains(line, leak) {
			t.Errorf("log %q contains %q", line, leak)
		}
	}
}

func TestRecovererRepanicsOnAbort(t *testing.T) {
	t.Parallel()

	handler := recoverer(slog.New(slog.DiscardHandler))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	defer func() {
		if rec := recover(); rec != http.ErrAbortHandler {
			t.Errorf("recover() = %v, want http.ErrAbortHandler", rec)
		}
	}()
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}
