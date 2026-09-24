package openmeteo_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dev-labs-ai/weather-panel/internal/openmeteo"
)

const recifeResult = `{"id":3390760,"name":"Recife","latitude":-8.05389,"longitude":-34.88111,` +
	`"country":"Brasil","admin1":"Pernambuco","admin2":"Recife","timezone":"America/Recife"}`

const recifeCurrent = `{"latitude":-8.049209,"longitude":-34.923065,"current":{"time":"2026-09-24T04:15",` +
	`"interval":900,"temperature_2m":25.3,"apparent_temperature":29.1,"relative_humidity_2m":77,` +
	`"weather_code":3,"wind_speed_10m":3.9}}`

// serve starts a server that answers every request with the status and body, and returns a client pointed at it for
// both APIs.
func serve(t *testing.T, status int, body string) *openmeteo.Client {
	t.Helper()
	return serveFunc(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
}

func serveFunc(t *testing.T, handler http.HandlerFunc) *openmeteo.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return openmeteo.NewClient(srv.URL+"/v1/search", srv.URL+"/v1/forecast")
}

func TestSearchSendsTheDocumentedQuery(t *testing.T) {
	t.Parallel()

	requests := make(chan *url.URL, 1)
	client := serveFunc(t, func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL
		_, _ = w.Write([]byte(`{"results":[` + recifeResult + `]}`))
	})
	if _, err := client.Search(t.Context(), "São Paulo"); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	got := <-requests

	if got.Path != "/v1/search" {
		t.Errorf("path = %q, want /v1/search", got.Path)
	}
	want := url.Values{"name": {"São Paulo"}, "count": {"1"}, "language": {"pt"}, "format": {"json"}}
	if q := got.Query(); q.Encode() != want.Encode() {
		t.Errorf("query = %v, want %v", q, want)
	}
}

func TestSearch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want openmeteo.Place
	}{
		{
			name: "every field",
			body: `{"results":[` + recifeResult + `],"generationtime_ms":0.6}`,
			want: openmeteo.Place{
				ID: 3390760, Name: "Recife", Admin1: "Pernambuco", Country: "Brasil",
				Latitude: -8.05389, Longitude: -34.88111,
			},
		},
		{
			name: "no admin1",
			body: `{"results":[{"id":2993458,"name":"Mônaco","latitude":43.73333,"longitude":7.41667,"country":"Mônaco"}]}`,
			want: openmeteo.Place{ID: 2993458, Name: "Mônaco", Country: "Mônaco", Latitude: 43.73333, Longitude: 7.41667},
		},
		{
			name: "no country",
			body: `{"results":[{"id":1,"name":"Ilha","latitude":1.5,"longitude":2.5,"admin1":"Região"}]}`,
			want: openmeteo.Place{ID: 1, Name: "Ilha", Admin1: "Região", Latitude: 1.5, Longitude: 2.5},
		},
		{
			name: "coordinates at zero",
			body: `{"results":[{"id":2,"name":"Null Island","latitude":0,"longitude":0}]}`,
			want: openmeteo.Place{ID: 2, Name: "Null Island"},
		},
		{
			name: "only the first of several results",
			body: `{"results":[` + recifeResult + `,{"id":9,"name":"Outra","latitude":1,"longitude":1}]}`,
			want: openmeteo.Place{
				ID: 3390760, Name: "Recife", Admin1: "Pernambuco", Country: "Brasil",
				Latitude: -8.05389, Longitude: -34.88111,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := serve(t, http.StatusOK, tt.body).Search(t.Context(), "Recife")
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Search() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSearchNotFound(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"missing results": `{"generationtime_ms":0.09679794}`,
		"empty results":   `{"results":[],"generationtime_ms":0.1}`,
		"null results":    `{"results":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := serve(t, http.StatusOK, body).Search(t.Context(), "Xyzzyqqq")
			if !errors.Is(err, openmeteo.ErrNotFound) {
				t.Errorf("Search() error = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestSearchUpstreamErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   int
		body     string
		wantText string
	}{
		{"400 with a reason", http.StatusBadRequest, `{"reason":"Parameter count must be between 1 and 100.","error":true}`,
			"status 400: Parameter count must be between 1 and 100."},
		{"400 without a body", http.StatusBadRequest, ``, "status 400"},
		{"500", http.StatusInternalServerError, `<html>Internal Server Error</html>`, "status 500"},
		{"429", http.StatusTooManyRequests, `{"reason":"Too many requests","error":true}`, "status 429: Too many requests"},
		{"malformed JSON", http.StatusOK, `{"results":[`, "decode response"},
		{"HTML instead of JSON", http.StatusOK, `<html></html>`, "decode response"},
		{"wrong field type", http.StatusOK, `{"results":[{"id":"abc","name":"Recife","latitude":1,"longitude":2}]}`,
			"decode response"},
		{"missing id", http.StatusOK, `{"results":[{"name":"Recife","latitude":1,"longitude":2}]}`, "no id"},
		{"missing name", http.StatusOK, `{"results":[{"id":1,"latitude":1,"longitude":2}]}`, "no name"},
		{"empty name", http.StatusOK, `{"results":[{"id":1,"name":"","latitude":1,"longitude":2}]}`, "no name"},
		{"missing latitude", http.StatusOK, `{"results":[{"id":1,"name":"Recife","longitude":2}]}`, "no latitude"},
		{"missing longitude", http.StatusOK, `{"results":[{"id":1,"name":"Recife","latitude":1}]}`, "no longitude"},
		{"null latitude", http.StatusOK, `{"results":[{"id":1,"name":"Recife","latitude":null,"longitude":2}]}`,
			"no latitude"},
		{"body over 1 MiB", http.StatusOK, `{"results":[` + recifeResult + `],"padding":"` +
			strings.Repeat("x", 1<<20) + `"}`, "larger than"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := serve(t, tt.status, tt.body).Search(t.Context(), "Recife")
			if !errors.Is(err, openmeteo.ErrUpstream) {
				t.Fatalf("Search() error = %v, want ErrUpstream", err)
			}
			if errors.Is(err, openmeteo.ErrNotFound) || errors.Is(err, openmeteo.ErrTimeout) {
				t.Errorf("Search() error = %v, want only ErrUpstream", err)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("Search() error = %q, want it to contain %q", err, tt.wantText)
			}
		})
	}
}

func TestSearchNetworkErrorOmitsTheURL(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close()
	client := openmeteo.NewClient(base+"/v1/search", base+"/v1/forecast")

	_, err := client.Search(t.Context(), "Recife")
	if !errors.Is(err, openmeteo.ErrUpstream) {
		t.Fatalf("Search() error = %v, want ErrUpstream", err)
	}
	if strings.Contains(err.Error(), "Recife") || strings.Contains(err.Error(), "name=") {
		t.Errorf("Search() error = %q, want it to omit the request URL", err)
	}
}

func TestSearchTimeout(t *testing.T) {
	t.Parallel()

	client := serveFunc(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := client.Search(ctx, "Recife")
	if !errors.Is(err, openmeteo.ErrTimeout) {
		t.Fatalf("Search() error = %v, want ErrTimeout", err)
	}
	if errors.Is(err, openmeteo.ErrUpstream) {
		t.Errorf("Search() error = %v, want only ErrTimeout", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Search() returned after %v, want it cut off at the deadline", elapsed)
	}
	if strings.Contains(err.Error(), "Recife") {
		t.Errorf("Search() error = %q, want it to omit the request URL", err)
	}
}

func TestSearchTimeoutWhileReadingTheBody(t *testing.T) {
	t.Parallel()

	client := serveFunc(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[`))
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	if _, err := client.Search(ctx, "Recife"); !errors.Is(err, openmeteo.ErrTimeout) {
		t.Errorf("Search() error = %v, want ErrTimeout", err)
	}
}

func TestCurrentSendsTheDocumentedQuery(t *testing.T) {
	t.Parallel()

	requests := make(chan *url.URL, 1)
	client := serveFunc(t, func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL
		_, _ = w.Write([]byte(recifeCurrent))
	})
	if _, err := client.Current(t.Context(), -8.05389, -34.88111); err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	got := <-requests

	if got.Path != "/v1/forecast" {
		t.Errorf("path = %q, want /v1/forecast", got.Path)
	}
	want := url.Values{
		"latitude":         {"-8.05389"},
		"longitude":        {"-34.88111"},
		"current":          {"temperature_2m,apparent_temperature,relative_humidity_2m,weather_code,wind_speed_10m"},
		"temperature_unit": {"celsius"},
		"wind_speed_unit":  {"kmh"},
	}
	if q := got.Query(); q.Encode() != want.Encode() {
		t.Errorf("query = %v, want %v", q, want)
	}
}

func TestCurrent(t *testing.T) {
	t.Parallel()

	got, err := serve(t, http.StatusOK, recifeCurrent).Current(t.Context(), -8.05389, -34.88111)
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	want := openmeteo.Current{
		TemperatureC:         25.3,
		ApparentTemperatureC: 29.1,
		RelativeHumidityPct:  77,
		WindSpeedKmh:         3.9,
		WeatherCode:          3,
	}
	if got != want {
		t.Errorf("Current() = %+v, want %+v", got, want)
	}
}

func TestCurrentUpstreamErrors(t *testing.T) {
	t.Parallel()

	const complete = `"temperature_2m":25.3,"apparent_temperature":29.1,"relative_humidity_2m":77,` +
		`"weather_code":3,"wind_speed_10m":3.9`
	without := func(field string) string {
		var kept []string
		for part := range strings.SplitSeq(complete, ",") {
			if !strings.HasPrefix(part, `"`+field+`"`) {
				kept = append(kept, part)
			}
		}
		return `{"current":{` + strings.Join(kept, ",") + `}}`
	}

	tests := []struct {
		name     string
		status   int
		body     string
		wantText string
	}{
		{"400 with a reason", http.StatusBadRequest, `{"reason":"Latitude must be in range of -90 to 90°.","error":true}`,
			"status 400: Latitude must be in range of -90 to 90°."},
		{"500", http.StatusInternalServerError, ``, "status 500"},
		{"503", http.StatusServiceUnavailable, `upstream down`, "status 503"},
		{"malformed JSON", http.StatusOK, `{"current":`, "decode response"},
		{"fractional weather code", http.StatusOK, `{"current":{` + strings.Replace(complete, `"weather_code":3`,
			`"weather_code":3.5`, 1) + `}}`, "decode response"},
		{"missing current", http.StatusOK, `{"latitude":-8.05,"longitude":-34.9}`, "no current"},
		{"null current", http.StatusOK, `{"current":null}`, "no current"},
		{"missing temperature_2m", http.StatusOK, without("temperature_2m"), "no current.temperature_2m"},
		{"missing apparent_temperature", http.StatusOK, without("apparent_temperature"),
			"no current.apparent_temperature"},
		{"missing relative_humidity_2m", http.StatusOK, without("relative_humidity_2m"),
			"no current.relative_humidity_2m"},
		{"missing weather_code", http.StatusOK, without("weather_code"), "no current.weather_code"},
		{"missing wind_speed_10m", http.StatusOK, without("wind_speed_10m"), "no current.wind_speed_10m"},
		{"null temperature_2m", http.StatusOK, `{"current":{` + strings.Replace(complete, "25.3", "null", 1) + `}}`,
			"no current.temperature_2m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := serve(t, tt.status, tt.body).Current(t.Context(), -8.05389, -34.88111)
			if !errors.Is(err, openmeteo.ErrUpstream) {
				t.Fatalf("Current() error = %v, want ErrUpstream", err)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("Current() error = %q, want it to contain %q", err, tt.wantText)
			}
		})
	}
}

func TestCurrentTimeout(t *testing.T) {
	t.Parallel()

	client := serveFunc(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	_, err := client.Current(ctx, -8.05389, -34.88111)
	if !errors.Is(err, openmeteo.ErrTimeout) {
		t.Fatalf("Current() error = %v, want ErrTimeout", err)
	}
	if strings.Contains(err.Error(), "-8.05389") {
		t.Errorf("Current() error = %q, want it to omit the coordinates", err)
	}
}
