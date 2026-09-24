package web_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/dev-labs-ai/weather-panel/internal/web"
)

func getIndex(t *testing.T) (*httptest.ResponseRecorder, doc) {
	t.Helper()
	rec := httptest.NewRecorder()
	router := web.NewRouter(slog.New(slog.DiscardHandler), testStatic)
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", rec.Code, http.StatusOK)
	}
	return rec, parseHTML(t, rec.Body.String())
}

func TestIndexRendersThePage(t *testing.T) {
	t.Parallel()
	rec, d := getIndex(t)

	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", got)
	}
	if got := attr(d.one("html"), "lang"); got != "pt-BR" {
		t.Errorf(`<html lang> = %q, want "pt-BR"`, got)
	}
	if got := text(d.one("title")); got != "Clima agora" {
		t.Errorf("<title> = %q, want %q", got, "Clima agora")
	}
	if got := text(d.one("h1")); got != "Clima agora" {
		t.Errorf("<h1> = %q, want %q", got, "Clima agora")
	}
	if !within(d.one("h1"), d.one("main")) {
		t.Error("the heading is outside <main>")
	}
}

func TestIndexLoadsSameOriginAssetsOnly(t *testing.T) {
	t.Parallel()
	_, d := getIndex(t)

	var stylesheets, scripts []string
	for _, link := range d.tags("link") {
		if attr(link, "rel") == "stylesheet" {
			stylesheets = append(stylesheets, attr(link, "href"))
		}
	}
	for _, script := range d.tags("script") {
		src := attr(script, "src")
		if src == "" {
			t.Errorf("found an inline <script>, which the CSP blocks: %q", text(script))
		}
		scripts = append(scripts, src)
	}
	if len(stylesheets) != 1 || !strings.HasPrefix(stylesheets[0], "/static/css/app.css") {
		t.Errorf("stylesheets = %q, want only /static/css/app.css", stylesheets)
	}
	if len(scripts) != 1 || !strings.HasPrefix(scripts[0], "/static/js/htmx.min.js") {
		t.Errorf("scripts = %q, want only /static/js/htmx.min.js", scripts)
	}
	for _, n := range all(d.root, func(n *html.Node) bool { return true }) {
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

	form := d.one("form")
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
		if got := attr(form, name); got != want {
			t.Errorf("<form %s> = %q, want %q", name, got, want)
		}
	}

	label := d.one("label")
	if attr(label, "for") != "city" || text(label) != "Cidade" {
		t.Errorf(`<label> = for %q, %q, want a persistent "Cidade" label for the city input`, attr(label, "for"),
			text(label))
	}
	input := d.byID("city")
	for name, want := range map[string]string{
		"name":      "city",
		"type":      "text",
		"maxlength": "100",
		"value":     "",
	} {
		if got := attr(input, name); got != want {
			t.Errorf("<input %s> = %q, want %q", name, got, want)
		}
	}
	if got := strings.Fields(attr(input, "aria-describedby")); !slices.Equal(got, []string{"city-hint", "city-error"}) {
		t.Errorf("<input aria-describedby> = %q, want the hint and the validation message", got)
	}
	if got := text(d.byID("city-hint")); got != "Digite o nome de uma cidade, por exemplo: Recife." {
		t.Errorf("hint = %q", got)
	}
	// The server is the single source of validation messages.
	for _, constraint := range []string{"required", "minlength", "pattern", "disabled"} {
		if hasAttr(input, constraint) {
			t.Errorf("<input> has %q, want no constraint besides maxlength", constraint)
		}
	}

	button := d.one("button")
	if attr(button, "type") != "submit" || text(button) != "Buscar" {
		t.Errorf("<button> = type %q, %q, want a submit button labeled Buscar", attr(button, "type"), text(button))
	}
	if !within(input, form) || !within(button, form) {
		t.Error("the input and the button must be inside the form")
	}
}

func TestIndexDisablesOnlyTheButtonDuringARequest(t *testing.T) {
	t.Parallel()
	_, d := getIndex(t)

	// hx-disable="find button" disables the first <button> inside the form; the input must never be the target.
	form := d.one("form")
	if got := attr(form, "hx-disable"); got != "find button" {
		t.Fatalf(`<form hx-disable> = %q, want "find button"`, got)
	}
	buttons := all(form, func(n *html.Node) bool { return n.Data == "button" })
	if len(buttons) != 1 {
		t.Fatalf("form holds %d buttons, want exactly the submit button", len(buttons))
	}
	if d.byID("city").Data != "input" {
		t.Error("the city field is not an <input>")
	}
}

func TestIndexLoadingIndicator(t *testing.T) {
	t.Parallel()
	_, d := getIndex(t)

	loading := d.byID("loading")
	if !slices.Contains(strings.Fields(attr(loading, "class")), "htmx-indicator") {
		t.Errorf(`#loading class = %q, want htmx-indicator`, attr(loading, "class"))
	}
	if attr(loading, "role") != "status" {
		t.Errorf(`#loading role = %q, want "status"`, attr(loading, "role"))
	}
	if got := text(loading); got != "Buscando o clima…" {
		t.Errorf("#loading text = %q, want %q", got, "Buscando o clima…")
	}
}

func TestIndexResultRegion(t *testing.T) {
	t.Parallel()
	_, d := getIndex(t)

	result := d.byID("result")
	if attr(result, "aria-live") != "polite" || attr(result, "aria-atomic") != "true" {
		t.Errorf(`#result aria-live = %q, aria-atomic = %q, want "polite" and "true"`, attr(result, "aria-live"),
			attr(result, "aria-atomic"))
	}
	if within(result, d.one("form")) {
		t.Error("#result is inside the form; swapping it would replace the input")
	}
	if !within(result, d.one("main")) {
		t.Error("#result is outside <main>")
	}
	if got := text(result); got != "" {
		t.Errorf("#result = %q, want it empty before a search", got)
	}
	if d.hasID("city-error") {
		t.Error("the page shows a validation message before any search")
	}
}
