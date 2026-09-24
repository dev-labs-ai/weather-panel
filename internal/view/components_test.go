package view_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"golang.org/x/net/html"

	"github.com/dev-labs-ai/weather-panel/internal/htmltest"
	"github.com/dev-labs-ai/weather-panel/internal/view"
	"github.com/dev-labs-ai/weather-panel/internal/weather"
)

var recife = weather.Report{
	Location: weather.Location{
		Name: "Recife", Admin1: "Pernambuco", Country: "Brasil", Latitude: -8.05389, Longitude: -34.88111,
	},
	Conditions: weather.Conditions{
		TemperatureC: 25.3, ApparentTemperatureC: 29.1, RelativeHumidityPct: 77, WindSpeedKmh: 3.9, WeatherCode: 3,
	},
}

func render(t *testing.T, c templ.Component) (string, htmltest.Doc) {
	t.Helper()
	var b strings.Builder
	if err := c.Render(t.Context(), &b); err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	return b.String(), htmltest.Parse(t, b.String())
}

// reading is a label and its value, as shown in a <dt> and the <dd> after it.
type reading struct{ label, value string }

// readings returns every <dt>/<dd> pair in document order. Values keep their no-break spaces.
func readings(d htmltest.Doc) []reading {
	var out []reading
	for _, dt := range d.Tags("dt") {
		dd := dt.NextSibling
		for dd != nil && dd.Type != html.ElementNode {
			dd = dd.NextSibling
		}
		value := ""
		if dd != nil && dd.Data == "dd" && dd.FirstChild != nil {
			value = strings.TrimSpace(dd.FirstChild.Data)
		}
		out = append(out, reading{htmltest.Text(dt), value})
	}
	return out
}

func TestResultShowsTheLocationAndEveryReading(t *testing.T) {
	t.Parallel()
	_, d := render(t, view.Result(recife))

	if got := htmltest.Text(d.One("h2")); got != "Agora em Recife, Pernambuco, Brasil" {
		t.Errorf("heading = %q, want the resolved location", got)
	}
	want := []reading{
		{"Temperatura", "25 °C"},
		{"Condição", "Nublado"},
		{"Sensação térmica", "29 °C"},
		{"Umidade", "77%"},
		{"Vento", "4 km/h"},
	}
	got := readings(d)
	if len(got) != len(want) {
		t.Fatalf("readings = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("reading %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResultHighlightsTemperatureAndCondition(t *testing.T) {
	t.Parallel()
	_, d := render(t, view.Result(recife))

	lists := d.Tags("dl")
	if len(lists) != 2 {
		t.Fatalf("found %d <dl>, want the highlighted pair and the other readings", len(lists))
	}
	var first []string
	for _, dt := range htmltest.All(lists[0], func(n *html.Node) bool { return n.Data == "dt" }) {
		first = append(first, htmltest.Text(dt))
	}
	if strings.Join(first, ",") != "Temperatura,Condição" {
		t.Errorf("highlighted readings = %q, want temperature and condition", first)
	}
}

func TestResultEndsWithTheAttribution(t *testing.T) {
	t.Parallel()
	_, d := render(t, view.Result(recife))

	article := d.One("article")
	var last *html.Node
	for c := article.LastChild; c != nil; c = c.PrevSibling {
		if c.Type == html.ElementNode {
			last = c
			break
		}
	}
	if last == nil || last.Data != "p" || htmltest.Text(last) != "Dados meteorológicos por Open-Meteo.com" {
		t.Fatalf("last element = %v, want the Open-Meteo attribution", last)
	}
	links := htmltest.All(last, func(n *html.Node) bool { return n.Data == "a" })
	if len(links) != 1 || htmltest.Attr(links[0], "href") != "https://open-meteo.com/" ||
		htmltest.Text(links[0]) != "Open-Meteo.com" {
		t.Errorf("attribution links = %v, want one link to https://open-meteo.com/", links)
	}
}

func TestResultLocationSkipsMissingParts(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		location weather.Location
		want     string
	}{
		{weather.Location{Name: "Mônaco", Country: "Mônaco"}, "Agora em Mônaco, Mônaco"},
		{weather.Location{Name: "Recife", Admin1: "Pernambuco"}, "Agora em Recife, Pernambuco"},
		{weather.Location{Name: "Atlântida"}, "Agora em Atlântida"},
	} {
		report := recife
		report.Location = tt.location
		_, d := render(t, view.Result(report))
		if got := htmltest.Text(d.One("h2")); got != tt.want {
			t.Errorf("heading = %q, want %q", got, tt.want)
		}
	}
}

func TestResultRoundsAndDescribesEdgeValues(t *testing.T) {
	t.Parallel()

	report := recife
	report.Conditions = weather.Conditions{
		TemperatureC: -0.4, ApparentTemperatureC: -7.5, RelativeHumidityPct: 99.6, WindSpeedKmh: 0.2, WeatherCode: 42,
	}
	_, d := render(t, view.Result(report))
	want := []reading{
		{"Temperatura", "0 °C"},
		{"Condição", "Condição não informada"},
		{"Sensação térmica", "-8 °C"},
		{"Umidade", "100%"},
		{"Vento", "0 km/h"},
	}
	got := readings(d)
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Errorf("readings = %q, want %q", got, want)
			break
		}
	}
}

func TestResultEscapesProviderText(t *testing.T) {
	t.Parallel()

	report := recife
	report.Location.Name = `<script>alert(1)</script>`
	body, d := render(t, view.Result(report))
	if len(d.Tags("script")) != 0 || strings.Contains(body, "<script>") {
		t.Errorf("provider text was rendered as markup:\n%s", body)
	}
}

func TestMessages(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		component templ.Component
		title     string
		text      string
	}{
		{
			"validation",
			view.ValidationMessage("Digite o nome de uma cidade para buscar o clima."),
			"Confira o nome da cidade",
			"Digite o nome de uma cidade para buscar o clima.",
		},
		{
			"not found",
			view.NotFoundMessage(),
			"Cidade não encontrada",
			"Não encontramos nenhuma cidade com esse nome. Confira a grafia ou tente uma cidade próxima e busque de novo.",
		},
		{
			"unavailable",
			view.UnavailableMessage(),
			"Clima indisponível no momento",
			"Não conseguimos obter os dados meteorológicos agora. Aguarde alguns instantes e busque de novo.",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, d := render(t, tt.component)

			// A heading names what went wrong, and the text says what to do next, both as plain text.
			if got := htmltest.Text(d.One("h2")); got != tt.title {
				t.Errorf("heading = %q, want %q", got, tt.title)
			}
			if got := htmltest.Text(d.One("p")); got != tt.text {
				t.Errorf("text = %q, want %q", got, tt.text)
			}
			all := htmltest.Text(d.Root)
			if regexp.MustCompile(`\d{3}`).MatchString(all) {
				t.Errorf("message %q shows a status code", all)
			}
			for _, internal := range []string{"Open-Meteo", "HTTP", "erro", "timeout", "upstream", "API"} {
				if strings.Contains(strings.ToLower(all), strings.ToLower(internal)) {
					t.Errorf("message %q uses the internal term %q", all, internal)
				}
			}
			if len(d.Tags("img"))+len(d.Tags("svg")) != 0 {
				t.Error("message uses an image; it must be text")
			}
		})
	}
}

func TestValidationMessageIsTheInputDescription(t *testing.T) {
	t.Parallel()
	_, d := render(t, view.ValidationMessage("O nome da cidade pode ter no máximo 100 caracteres."))

	if got := htmltest.Text(d.ByID("city-error")); got != "O nome da cidade pode ter no máximo 100 caracteres." {
		t.Errorf("#city-error = %q, want the validation message alone", got)
	}
}

func TestPageWithAResult(t *testing.T) {
	t.Parallel()
	_, d := render(t, view.Page("Recife", view.Result(recife)))

	if got := htmltest.Attr(d.ByID("city"), "value"); got != "Recife" {
		t.Errorf("input value = %q, want the searched city", got)
	}
	article := d.One("article")
	if !htmltest.Within(article, d.ByID("result")) {
		t.Error("the result is outside #result")
	}
}
