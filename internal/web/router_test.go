package web_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dev-labs-ai/weather-panel/internal/web"
)

func TestResponsesCarrySecurityHeaders(t *testing.T) {
	t.Parallel()

	router := web.NewRouter(slog.New(slog.DiscardHandler))
	for _, tt := range []struct {
		name, method, target string
		wantStatus           int
	}{
		{"health check", http.MethodGet, "/healthz", http.StatusOK},
		{"unknown route", http.MethodGet, "/missing", http.StatusNotFound},
		{"wrong method", http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.target, nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			for name, want := range map[string]string{
				"Content-Security-Policy": "default-src 'self'; script-src 'self'; style-src 'self'; " +
					"img-src 'self' data:; connect-src 'self'; form-action 'self'; base-uri 'none'; " +
					"frame-ancestors 'none'",
				"X-Content-Type-Options": "nosniff",
				"Referrer-Policy":        "no-referrer",
			} {
				if got := rec.Header().Get(name); got != want {
					t.Errorf("%s = %q, want %q", name, got, want)
				}
			}
		})
	}
}

func TestHealthz(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	web.NewRouter(slog.New(slog.DiscardHandler)).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "ok\n" {
		t.Errorf("body = %q, want %q", got, "ok\n")
	}
}

func TestRequestLogOmitsQueryAndClientAddress(t *testing.T) {
	t.Parallel()

	for _, target := range []string{"/healthz?city=Recife", "/weather?city=Recife"} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()

			var logs bytes.Buffer
			router := web.NewRouter(slog.New(slog.NewJSONHandler(&logs, nil)))
			req := httptest.NewRequest(http.MethodGet, target, nil)
			req.RemoteAddr = "203.0.113.7:51234"
			req.Header.Set("X-Forwarded-For", "198.51.100.9")
			router.ServeHTTP(httptest.NewRecorder(), req)

			line := logs.String()
			for _, leak := range []string{"Recife", "city", "203.0.113.7", "198.51.100.9"} {
				if strings.Contains(line, leak) {
					t.Errorf("log %q contains %q", line, leak)
				}
			}

			var entry map[string]any
			if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
				t.Fatalf("log line is not one JSON object: %v\n%s", err, line)
			}
			for _, key := range []string{"method", "route", "status", "duration_ms"} {
				if _, ok := entry[key]; !ok {
					t.Errorf("log line has no %q: %s", key, line)
				}
			}
		})
	}
}

func TestRequestLogRecordsRoutePattern(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	router := web.NewRouter(slog.New(slog.NewJSONHandler(&logs, nil)))
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz?city=Recife", nil))

	var entry struct {
		Method string `json:"method"`
		Route  string `json:"route"`
		Status int    `json:"status"`
	}
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatalf("decode log line: %v", err)
	}
	if entry.Method != http.MethodGet || entry.Route != "/healthz" || entry.Status != http.StatusOK {
		t.Errorf("log = %+v, want GET /healthz 200", entry)
	}
}
