package web_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/dev-labs-ai/weather-panel/internal/htmltest"
	"github.com/dev-labs-ai/weather-panel/internal/web"
)

func getIndex(t *testing.T) (*httptest.ResponseRecorder, htmltest.Doc) {
	t.Helper()
	rec := httptest.NewRecorder()
	router := web.NewRouter(slog.New(slog.DiscardHandler), testStatic, stubLookup{})
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", rec.Code, http.StatusOK)
	}
	return rec, htmltest.Parse(t, rec.Body.String())
}

func TestIndexRendersThePage(t *testing.T) {
	t.Parallel()
	rec, d := getIndex(t)

	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", got)
	}
	if got := htmltest.Attr(d.One("html"), "lang"); got != "pt-BR" {
		t.Errorf(`<html lang> = %q, want "pt-BR"`, got)
	}
	if got := htmltest.Text(d.One("title")); got != "Clima agora" {
		t.Errorf("<title> = %q, want %q", got, "Clima agora")
	}
	if got := htmltest.Text(d.One("h1")); got != "Clima agora" {
		t.Errorf("<h1> = %q, want %q", got, "Clima agora")
	}
	if !htmltest.Within(d.One("h1"), d.One("main")) {
		t.Error("the heading is outside <main>")
	}
}

func TestIndexLoadsSameOriginAssetsOnly(t *testing.T) {
	t.Parallel()
	_, d := getIndex(t)

	var stylesheets, scripts []string
	for _, link := range d.Tags("link") {
		if htmltest.Attr(link, "rel") == "stylesheet" {
			stylesheets = append(stylesheets, htmltest.Attr(link, "href"))
		}
	}
	for _, script := range d.Tags("script") {
		src := htmltest.Attr(script, "src")
		if src == "" {
			t.Errorf("found an inline <script>, which the CSP blocks: %q", htmltest.Text(script))
		}
		scripts = append(scripts, src)
	}
	if len(stylesheets) != 1 || !strings.HasPrefix(stylesheets[0], "/static/css/app.css") {
		t.Errorf("stylesheets = %q, want only /static/css/app.css", stylesheets)
	}
	if len(scripts) != 1 || !strings.HasPrefix(scripts[0], "/static/js/htmx.min.js") {
		t.Errorf("scripts = %q, want only /static/js/htmx.min.js", scripts)
	}
	for _, n := range htmltest.All(d.Root, func(n *html.Node) bool { return true }) {
		for _, a := range n.Attr {
			if (a.Key == "src" || a.Key == "href") && !strings.HasPrefix(a.Val, "/") && a.Val != "data:," {
				t.Errorf("<%s %s=%q> loads a resource from another origin", n.Data, a.Key, a.Val)
			}
			if strings.HasPrefix(a.Key, "on") || strings.HasPrefix(a.Key, "hx-on") {
				t.Errorf("<%s> has the inline handler %s, which the CSP blocks", n.Data, a.Key)
			}
		}
	}
}

func TestIndexSearchForm(t *testing.T) {
	t.Parallel()
	_, d := getIndex(t)

	form := d.One("form")
	for name, want := range map[string]string{
		"action":       "/weather",
		"method":       "get",
		"role":         "search",
		"hx-get":       "/weather",
		"hx-target":    "#result",
		"hx-swap":      "innerHTML",
		"hx-disable":   "find button",
		"hx-sync":      "this:drop",
		"hx-indicator": "#loading",
	} {
		if got := htmltest.Attr(form, name); got != want {
			t.Errorf("<form %s> = %q, want %q", name, got, want)
		}
	}

	label := d.One("label")
	if htmltest.Attr(label, "for") != "city" || htmltest.Text(label) != "Cidade" {
		t.Errorf(`<label> = for %q, %q, want a persistent "Cidade" label for the city input`, htmltest.Attr(label, "for"),
			htmltest.Text(label))
	}
	input := d.ByID("city")
	for name, want := range map[string]string{
		"name":      "city",
		"type":      "text",
		"maxlength": "100",
		"value":     "",
	} {
		if got := htmltest.Attr(input, name); got != want {
			t.Errorf("<input %s> = %q, want %q", name, got, want)
		}
	}
	describedBy := strings.Fields(htmltest.Attr(input, "aria-describedby"))
	if got := describedBy; !slices.Equal(got, []string{"city-hint", "city-error"}) {
		t.Errorf("<input aria-describedby> = %q, want the hint and the validation message", got)
	}
	if got := htmltest.Text(d.ByID("city-hint")); got != "Digite o nome de uma cidade, por exemplo: Recife." {
		t.Errorf("hint = %q", got)
	}
	// The server is the single source of validation messages.
	for _, constraint := range []string{"required", "minlength", "pattern", "disabled"} {
		if htmltest.HasAttr(input, constraint) {
			t.Errorf("<input> has %q, want no constraint besides maxlength", constraint)
		}
	}

	button := d.One("button")
	if htmltest.Attr(button, "type") != "submit" || htmltest.Text(button) != "Buscar" {
		t.Errorf("<button> = type %q, %q, want a submit button labeled Buscar", htmltest.Attr(button, "type"),
			htmltest.Text(button))
	}
	if !htmltest.Within(input, form) || !htmltest.Within(button, form) {
		t.Error("the input and the button must be inside the form")
	}
}

func TestIndexDisablesOnlyTheButtonDuringARequest(t *testing.T) {
	t.Parallel()
	_, d := getIndex(t)

	// hx-disable="find button" disables the first <button> inside the form; the input must never be the target.
	form := d.One("form")
	if got := htmltest.Attr(form, "hx-disable"); got != "find button" {
		t.Fatalf(`<form hx-disable> = %q, want "find button"`, got)
	}
	buttons := htmltest.All(form, func(n *html.Node) bool { return n.Data == "button" })
	if len(buttons) != 1 {
		t.Fatalf("form holds %d buttons, want exactly the submit button", len(buttons))
	}
	if d.ByID("city").Data != "input" {
		t.Error("the city field is not an <input>")
	}
}

func TestIndexLoadingIndicator(t *testing.T) {
	t.Parallel()
	_, d := getIndex(t)

	loading := d.ByID("loading")
	if !slices.Contains(strings.Fields(htmltest.Attr(loading, "class")), "htmx-indicator") {
		t.Errorf(`#loading class = %q, want htmx-indicator`, htmltest.Attr(loading, "class"))
	}
	if htmltest.Attr(loading, "role") != "status" {
		t.Errorf(`#loading role = %q, want "status"`, htmltest.Attr(loading, "role"))
	}
	if got := htmltest.Text(loading); got != "Buscando o clima…" {
		t.Errorf("#loading text = %q, want %q", got, "Buscando o clima…")
	}
}

func TestIndexResultRegion(t *testing.T) {
	t.Parallel()
	_, d := getIndex(t)

	result := d.ByID("result")
	if htmltest.Within(result, d.One("form")) {
		t.Error("#result is inside the form; swapping it would replace the input")
	}
	if !htmltest.Within(result, d.One("main")) {
		t.Error("#result is outside <main>")
	}
	// Announcements go through #announcer; a live result region would announce its content piece by piece.
	if htmltest.HasAttr(result, "aria-live") || htmltest.HasAttr(result, "role") {
		t.Error("#result is a live region; announcements belong to #announcer")
	}
	if got := htmltest.Text(result); got != "" {
		t.Errorf("#result = %q, want it empty before a search", got)
	}
	if d.HasID("city-error") {
		t.Error("the page shows a validation message before any search")
	}
}

func TestIndexAnnouncer(t *testing.T) {
	t.Parallel()
	_, d := getIndex(t)

	announcer := d.ByID("announcer")
	if htmltest.Attr(announcer, "role") != "status" {
		t.Errorf(`#announcer role = %q, want "status"`, htmltest.Attr(announcer, "role"))
	}
	if !slices.Contains(strings.Fields(htmltest.Attr(announcer, "class")), "sr-only") {
		t.Errorf("#announcer class = %q, want it visually hidden with sr-only", htmltest.Attr(announcer, "class"))
	}
	// htmx fills it out of band; the element must survive every swap of #result.
	if htmltest.Within(announcer, d.ByID("result")) || htmltest.Within(announcer, d.One("form")) {
		t.Error("#announcer is inside a region that swaps replace")
	}
	if got := htmltest.Text(announcer); got != "" {
		t.Errorf("#announcer = %q, want it empty before a search", got)
	}
}
