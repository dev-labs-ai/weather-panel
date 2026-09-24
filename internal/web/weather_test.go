package web_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/dev-labs-ai/weather-panel/internal/htmltest"
	"github.com/dev-labs-ai/weather-panel/internal/openmeteo"
	"github.com/dev-labs-ai/weather-panel/internal/weather"
	"github.com/dev-labs-ai/weather-panel/internal/web"
)

// lookupTimeout is the service deadline in these tests; the fake server's slow answers take longer.
const lookupTimeout = 200 * time.Millisecond

// fakeOpenMeteo answers the Geocoding and Forecast APIs according to the searched name:
//
//	Recife     two places named Recife; the forecast for the first succeeds
//	Xyzzyqqq   no results
//	Falha      the Geocoding API fails with a 500
//	Quebrado   a place whose forecast is malformed
//	Lento      the Geocoding API answers after the lookup deadline
func fakeOpenMeteo(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path + "?" + q.Get("name") + q.Get("latitude") {
		case "/v1/search?Recife":
			_, _ = w.Write([]byte(`{"results":[` +
				`{"id":3390760,"name":"Recife","latitude":-8.05389,"longitude":-34.88111,"admin1":"Pernambuco","country":"Brasil"},` +
				`{"id":1,"name":"Recife","latitude":10,"longitude":10,"country":"Outro País"}]}`))
		case "/v1/search?Xyzzyqqq":
			_, _ = w.Write([]byte(`{"generationtime_ms":0.1}`))
		case "/v1/search?Falha":
			w.WriteHeader(http.StatusInternalServerError)
		case "/v1/search?Quebrado":
			_, _ = w.Write([]byte(`{"results":[{"id":2,"name":"Quebrado","latitude":2,"longitude":2}]}`))
		case "/v1/search?Lento":
			select {
			case <-r.Context().Done():
			case <-time.After(5 * lookupTimeout):
			}
		case "/v1/forecast?-8.05389":
			_, _ = w.Write([]byte(`{"current":{"temperature_2m":25.3,"apparent_temperature":29.1,` +
				`"relative_humidity_2m":77,"weather_code":3,"wind_speed_10m":3.9}}`))
		case "/v1/forecast?2":
			_, _ = w.Write([]byte(`{"current":`))
		default:
			t.Errorf("unexpected request to the fake Open-Meteo: %s", r.URL)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newApp returns the router wired to a real service and Open-Meteo client that talk to the fake server.
func newApp(t *testing.T) (http.Handler, *bytes.Buffer) {
	t.Helper()
	srv := fakeOpenMeteo(t)
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	client := openmeteo.NewClient(srv.URL+"/v1/search", srv.URL+"/v1/forecast")
	service := weather.NewService(client, weather.NopCache{}, lookupTimeout, logger)
	return web.NewRouter(logger, testStatic, service), &logs
}

func search(t *testing.T, app http.Handler, city string, htmx bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/weather?"+url.Values{"city": {city}}.Encode(), nil)
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	return rec
}

var outcomes = []struct {
	name, city string
	wantStatus int
	wantTitle  string // the result region's heading
	wantText   string // text the result region must contain
}{
	{"success", "Recife", http.StatusOK, "Agora em Recife, Pernambuco, Brasil", "Temperatura 25 °C Condição Nublado"},
	{"invalid input", "  a ", http.StatusUnprocessableEntity, "Confira o nome da cidade",
		"O nome da cidade precisa ter pelo menos duas letras ou números."},
	{"empty input", "", http.StatusUnprocessableEntity, "Confira o nome da cidade",
		"Digite o nome de uma cidade para buscar o clima."},
	{"not found", "Xyzzyqqq", http.StatusNotFound, "Cidade não encontrada", "Não encontramos nenhuma cidade"},
	{"upstream error", "Falha", http.StatusBadGateway, "Clima indisponível no momento", "busque de novo"},
	{"invalid upstream response", "Quebrado", http.StatusBadGateway, "Clima indisponível no momento",
		"busque de novo"},
	{"upstream timeout", "Lento", http.StatusGatewayTimeout, "Clima indisponível no momento", "busque de novo"},
}

func TestWeatherFragment(t *testing.T) {
	t.Parallel()

	for _, tt := range outcomes {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app, _ := newApp(t)

			rec := search(t, app, tt.city, true)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			assertHTMLResponse(t, rec)
			body := rec.Body.String()
			if strings.Contains(body, "<html") || strings.Contains(body, "<form") {
				t.Fatalf("fragment holds the whole page:\n%s", body)
			}
			d := htmltest.Parse(t, body)
			if got := htmltest.Text(d.One("h2")); got != tt.wantTitle {
				t.Errorf("heading = %q, want %q", got, tt.wantTitle)
			}
			if got := htmltest.Text(d.Root); !strings.Contains(got, tt.wantText) {
				t.Errorf("fragment text = %q, want it to contain %q", got, tt.wantText)
			}
		})
	}
}

func TestWeatherFullPage(t *testing.T) {
	t.Parallel()

	for _, tt := range outcomes {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app, _ := newApp(t)

			rec := search(t, app, tt.city, false)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			assertHTMLResponse(t, rec)
			d := htmltest.Parse(t, rec.Body.String())
			d.One("form")
			if got := htmltest.Attr(d.ByID("city"), "value"); got != tt.city {
				t.Errorf("input value = %q, want the typed %q", got, tt.city)
			}
			result := d.ByID("result")
			heading := htmltest.All(result, func(n *html.Node) bool { return n.Data == "h2" })
			if len(heading) != 1 || htmltest.Text(heading[0]) != tt.wantTitle {
				t.Errorf("#result headings = %v, want one %q", heading, tt.wantTitle)
			}
			if got := htmltest.Text(result); !strings.Contains(got, tt.wantText) {
				t.Errorf("#result text = %q, want it to contain %q", got, tt.wantText)
			}
		})
	}
}

func assertHTMLResponse(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", got)
	}
	if got := rec.Header().Values("Vary"); len(got) != 1 || got[0] != "HX-Request" {
		t.Errorf("Vary = %q, want HX-Request", got)
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Error("response has no Content-Security-Policy")
	}
}

func TestWeatherUsesTheFirstPlaceWithoutAskingToChoose(t *testing.T) {
	t.Parallel()
	app, _ := newApp(t)

	d := htmltest.Parse(t, search(t, app, "Recife", true).Body.String())
	text := htmltest.Text(d.Root)
	if strings.Contains(text, "Outro País") {
		t.Errorf("result mentions the second place: %q", text)
	}
	if len(d.Tags("form"))+len(d.Tags("select"))+len(d.Tags("input"))+len(d.Tags("button")) != 0 {
		t.Error("result asks the user to choose between places")
	}
	links := d.Tags("a")
	if len(links) != 1 || htmltest.Attr(links[0], "href") != "https://open-meteo.com/" {
		t.Errorf("links = %v, want the Open-Meteo attribution", links)
	}
}

// TestErrorReplacesAPreviousResult swaps two responses into the page's result region the way htmx does with
// hx-swap="innerHTML", and checks that nothing of the first result survives the error.
func TestErrorReplacesAPreviousResult(t *testing.T) {
	t.Parallel()
	app, _ := newApp(t)

	page := htmltest.Parse(t, search(t, app, "", false).Body.String())
	result := page.ByID("result")
	swap := func(fragment string) {
		t.Helper()
		nodes, err := html.ParseFragment(strings.NewReader(fragment), &html.Node{
			Type: html.ElementNode, Data: "section", DataAtom: atom.Section,
		})
		if err != nil {
			t.Fatalf("parse fragment: %v", err)
		}
		for c := result.FirstChild; c != nil; c = result.FirstChild {
			result.RemoveChild(c)
		}
		for _, n := range nodes {
			result.AppendChild(n)
		}
	}

	swap(search(t, app, "Recife", true).Body.String())
	if !strings.Contains(htmltest.Text(result), "Recife") {
		t.Fatalf("first search did not show Recife: %q", htmltest.Text(result))
	}

	for _, city := range []string{"Falha", "Lento", "Xyzzyqqq", "a"} {
		swap(search(t, app, "Recife", true).Body.String())
		swap(search(t, app, city, true).Body.String())

		got := htmltest.Text(result)
		for _, stale := range []string{"Recife", "Temperatura", "°C", "Open-Meteo"} {
			if strings.Contains(got, stale) {
				t.Errorf("after searching %q, #result = %q, still showing %q", city, got, stale)
			}
		}
		if len(htmltest.All(result, func(n *html.Node) bool { return n.Data == "article" })) != 0 {
			t.Errorf("after searching %q, #result still holds a result", city)
		}
	}
}

func TestWeatherLogsNeitherTheCityNorTheLocation(t *testing.T) {
	t.Parallel()
	app, logs := newApp(t)

	for _, tt := range outcomes {
		search(t, app, tt.city, true)
	}
	for _, leak := range []string{"Recife", "Pernambuco", "Xyzzyqqq", "Falha", "Quebrado", "Lento", "-8.05", "city="} {
		if strings.Contains(logs.String(), leak) {
			t.Errorf("logs contain %q:\n%s", leak, logs)
		}
	}
	if !strings.Contains(logs.String(), `"route":"/weather"`) {
		t.Errorf("logs do not record the /weather route:\n%s", logs)
	}
}
