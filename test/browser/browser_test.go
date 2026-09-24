//go:build browser

package browser_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/input"
	cdplog "github.com/chromedp/cdproto/log"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// baseURL is the running panel, normally the app service from compose.yaml.
var baseURL = func() string {
	if v := os.Getenv("BROWSER_TEST_URL"); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	return "http://localhost:8080"
}()

func TestMain(m *testing.M) {
	resp, err := http.Get(baseURL + "/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "the panel is not running at %s (%v); start it with `docker compose up -d --build` "+
			"or set BROWSER_TEST_URL\n", baseURL, err)
		os.Exit(1)
	}
	resp.Body.Close()
	os.Exit(m.Run())
}

// newTab starts a headless Chrome with the viewport and returns a context for driving it.
func newTab(t *testing.T, width, height int) context.Context {
	t.Helper()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.WindowSize(width, height))
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancelTab := chromedp.NewContext(allocCtx)
	ctx, cancelTimeout := context.WithTimeout(ctx, 90*time.Second)
	t.Cleanup(func() {
		cancelTimeout()
		cancelTab()
		cancelAlloc()
	})
	run(t, ctx, chromedp.EmulateViewport(int64(width), int64(height)))
	return ctx
}

func run(t *testing.T, ctx context.Context, actions ...chromedp.Action) {
	t.Helper()
	if err := chromedp.Run(ctx, actions...); err != nil {
		t.Fatalf("browser: %v", err)
	}
}

func eval[T any](t *testing.T, ctx context.Context, js string) T {
	t.Helper()
	var out T
	run(t, ctx, chromedp.Evaluate(js, &out))
	return out
}

// waitFor polls the JavaScript condition until it is true.
func waitFor(t *testing.T, ctx context.Context, condition string) {
	t.Helper()
	run(t, ctx, chromedp.Poll(condition, nil, chromedp.WithPollingTimeout(15*time.Second)))
}

// search types the city into the empty input and submits with Enter, like a keyboard user, then waits for the
// response to fill the result region. It returns the heading of what the region shows.
func search(t *testing.T, ctx context.Context, city string) string {
	t.Helper()
	run(t, ctx,
		chromedp.Evaluate(`document.getElementById("result").replaceChildren(); document.getElementById("city").value = ""`,
			nil),
		chromedp.Focus("#city", chromedp.ByID),
		chromedp.KeyEvent(city),
		chromedp.KeyEvent(kb.Enter),
	)
	waitFor(t, ctx, `document.querySelector("#result h2") !== null && !document.querySelector("form button").disabled`)
	return eval[string](t, ctx, `document.querySelector("#result h2").textContent.trim().replace(/\s+/g, " ")`)
}

// searchForResult searches until the panel shows a result. Open-Meteo may answer the first lookup after an idle
// period too slowly; the retry finds warm connections and, usually, a cached location.
func searchForResult(t *testing.T, ctx context.Context, city string) {
	t.Helper()
	for attempt := 1; ; attempt++ {
		heading := search(t, ctx, city)
		if strings.HasPrefix(heading, "Agora em") {
			return
		}
		if attempt == 3 {
			t.Fatalf("searching %q showed %q three times, want a result", city, heading)
		}
		t.Logf("searching %q showed %q; retrying", city, heading)
	}
}

func open(t *testing.T, ctx context.Context) {
	t.Helper()
	run(t, ctx, chromedp.Navigate(baseURL+"/"), chromedp.WaitVisible("#city", chromedp.ByID))
	waitFor(t, ctx, `typeof htmx === "object"`)
}

// AC-04: every request the browser makes during a search goes to the application's own origin.
func TestSearchRequestsStayOnTheApplicationOrigin(t *testing.T) {
	ctx := newTab(t, 1280, 800)
	app, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse %s: %v", baseURL, err)
	}

	var mu sync.Mutex
	var requests, violations []string
	chromedp.ListenTarget(ctx, func(ev any) {
		mu.Lock()
		defer mu.Unlock()
		switch ev := ev.(type) {
		case *network.EventRequestWillBeSent:
			requests = append(requests, ev.Request.URL)
		case *cdplog.EventEntryAdded:
			if strings.Contains(ev.Entry.Text, "Content Security Policy") {
				violations = append(violations, ev.Entry.Text)
			}
		}
	})
	run(t, ctx, network.Enable(), cdplog.Enable())

	open(t, ctx)
	search(t, ctx, "Recife")
	search(t, ctx, "Xyzzyqqq")
	search(t, ctx, "a")
	run(t, ctx, chromedp.Sleep(500*time.Millisecond)) // let late events arrive

	mu.Lock()
	defer mu.Unlock()
	lookups := 0
	for _, raw := range requests {
		u, err := url.Parse(raw)
		if err != nil {
			t.Errorf("unparsable request URL %q", raw)
			continue
		}
		if u.Scheme == "data" {
			continue // the inline empty icon; nothing leaves the browser
		}
		if u.Scheme != app.Scheme || u.Host != app.Host {
			t.Errorf("the browser requested %s, outside the application's origin %s", raw, baseURL)
		}
		if u.Path == "/weather" {
			lookups++
		}
	}
	if lookups != 3 {
		t.Errorf("saw %d lookups to /weather, want 3; requests: %q", lookups, requests)
	}
	for _, v := range violations {
		t.Errorf("Content Security Policy violation: %s", v)
	}
}

// AC-08: while a lookup is in flight the panel shows a loading state, the button is disabled, the input is not, and
// a second submission sends no second request.
func TestDoubleSubmissionSendsOneRequest(t *testing.T) {
	ctx := newTab(t, 1280, 800)

	paused := make(chan fetch.RequestID, 10)
	chromedp.ListenTarget(ctx, func(ev any) {
		if ev, ok := ev.(*fetch.EventRequestPaused); ok {
			paused <- ev.RequestID
		}
	})
	open(t, ctx)
	run(t, ctx, fetch.Enable().WithPatterns([]*fetch.RequestPattern{{URLPattern: "*/weather?*"}}))

	loadingVisible := `getComputedStyle(document.getElementById("loading")).visibility === "visible"`
	if eval[bool](t, ctx, loadingVisible) {
		t.Error("the loading message shows before any search")
	}
	run(t, ctx,
		chromedp.SetValue("#city", "Recife", chromedp.ByID),
		chromedp.Click("form button", chromedp.ByQuery),
	)
	var first fetch.RequestID
	select {
	case first = <-paused:
	case <-time.After(10 * time.Second):
		t.Fatal("the search sent no request")
	}

	// The request is held, so the lookup is in flight.
	if !eval[bool](t, ctx, `document.querySelector("form button").disabled`) {
		t.Error("the button is enabled while the lookup is in flight")
	}
	if eval[bool](t, ctx, `document.getElementById("city").disabled`) {
		t.Error("the input is disabled while the lookup is in flight")
	}
	waitFor(t, ctx, loadingVisible)
	if got := eval[string](t, ctx, `document.getElementById("loading").textContent.trim()`); got != "Buscando o clima…" {
		t.Errorf("loading message = %q", got)
	}

	// Try to submit again: a click on the disabled button, Enter in the input, and a scripted submission, which
	// htmx drops because of hx-sync="this:drop".
	run(t, ctx,
		chromedp.Click("form button", chromedp.ByQuery, chromedp.NodeReady),
		chromedp.Focus("#city", chromedp.ByID),
		chromedp.KeyEvent(kb.Enter),
		chromedp.Evaluate(`document.querySelector("form").requestSubmit()`, nil),
		chromedp.Sleep(500*time.Millisecond),
	)
	select {
	case id := <-paused:
		t.Errorf("a second submission sent request %s while the first was in flight", id)
		run(t, ctx, fetch.ContinueRequest(id))
	default:
	}

	run(t, ctx, fetch.ContinueRequest(first))
	waitFor(t, ctx, `document.querySelector("#result h2") !== null`)
	waitFor(t, ctx, `!document.querySelector("form button").disabled`)
	if eval[bool](t, ctx, loadingVisible) {
		t.Error("the loading message still shows after the lookup finished")
	}
	select {
	case id := <-paused:
		t.Errorf("a queued submission sent request %s after the first finished", id)
	case <-time.After(500 * time.Millisecond):
	}
}

// focusState describes the focused element and whether it shows a visible focus indicator.
const focusState = `(() => {
	const el = document.activeElement;
	const cs = getComputedStyle(el);
	const visible = el.matches(":focus-visible") && cs.outlineStyle !== "none" && parseFloat(cs.outlineWidth) >= 2;
	return (el.id ? "#" + el.id : el.tagName.toLowerCase() + ":" + el.textContent.trim()) + (visible ? " visible" : " HIDDEN");
})()`

// AC-10: a keyboard-only user reaches every control in a logical order, always sees where the focus is, and keeps
// the input focused after a search, so the next search needs no extra keystrokes.
func TestKeyboardOnlyFlow(t *testing.T) {
	ctx := newTab(t, 1280, 800)
	open(t, ctx)

	tab := chromedp.KeyEvent(kb.Tab)
	shiftTab := chromedp.KeyEvent(kb.Tab, chromedp.KeyModifiers(input.ModifierShift))
	focus := func(want string) {
		t.Helper()
		if got := eval[string](t, ctx, focusState); got != want {
			t.Errorf("focus = %q, want %q", got, want)
		}
	}

	run(t, ctx, tab)
	focus("#city visible")
	run(t, ctx, tab)
	focus("button:Buscar visible")
	run(t, ctx, shiftTab)
	focus("#city visible")

	searchForResult(t, ctx, "Recife")
	focus("#city visible")
	run(t, ctx, tab)
	focus("button:Buscar visible")
	run(t, ctx, tab)
	focus("a:Open-Meteo.com visible")

	heading := search(t, ctx, "a")
	if heading != "Confira o nome da cidade" {
		t.Fatalf("searching %q showed %q, want the validation message", "a", heading)
	}
	focus("#city visible")
	// The message is the input's description, so a screen reader reads it with the input.
	describedBy := eval[string](t, ctx, `document.getElementById("city").getAttribute("aria-describedby")
		.split(" ").map(id => document.getElementById(id)?.textContent.trim() ?? "").join(" | ")`)
	if !strings.Contains(describedBy, "pelo menos duas letras") {
		t.Errorf("input description = %q, want it to include the validation message", describedBy)
	}
}

// AC-12: at 360 px and 1280 px, no state of the panel scrolls horizontally.
func TestNoHorizontalScrolling(t *testing.T) {
	for _, width := range []int{360, 1280} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			ctx := newTab(t, width, 800)
			open(t, ctx)

			assertFits := func(state string) {
				t.Helper()
				sizes := eval[[]int](t, ctx,
					`[document.documentElement.scrollWidth, document.documentElement.clientWidth]`)
				if sizes[0] > sizes[1] {
					t.Errorf("%s at %d px: page is %d px wide in a %d px viewport", state, width, sizes[0], sizes[1])
				}
			}
			assertFits("empty panel")
			searchForResult(t, ctx, "Recife")
			assertFits("result")
			searchForResult(t, ctx, "Llanfairpwllgwyngyll")
			assertFits("result with a long place name")
			search(t, ctx, "Xyzzyqqq")
			assertFits("city not found")
			search(t, ctx, "a")
			assertFits("validation message")
		})
	}
}

// AC-11: each response reaches screen readers through the #announcer live region as one text, and a response that
// repeats the previous one is announced again: the previous text leaves the accessibility tree while the lookup is
// in flight, so the repeated text arrives as an addition.
func TestAnnouncerReadsEveryResponse(t *testing.T) {
	ctx := newTab(t, 1280, 800)

	paused := make(chan fetch.RequestID, 10)
	chromedp.ListenTarget(ctx, func(ev any) {
		if ev, ok := ev.(*fetch.EventRequestPaused); ok {
			paused <- ev.RequestID
		}
	})
	open(t, ctx)

	const notFound = "Cidade não encontrada. Não encontramos nenhuma cidade com esse nome. Confira a grafia ou tente " +
		"uma cidade próxima e busque de novo."
	announced := `(() => {
		const el = document.querySelector("#announcer > *");
		return el && getComputedStyle(el).display !== "none" ? el.textContent : "";
	})()`
	if got := eval[string](t, ctx, announced); got != "" {
		t.Fatalf("announcer = %q before any search, want it empty", got)
	}
	search(t, ctx, "Xyzzyqqq")
	if got := eval[string](t, ctx, announced); got != notFound {
		t.Fatalf("announcer = %q, want %q", got, notFound)
	}

	// Repeat the search and hold it in flight.
	run(t, ctx,
		fetch.Enable().WithPatterns([]*fetch.RequestPattern{{URLPattern: "*/weather?*"}}),
		chromedp.Focus("#city", chromedp.ByID),
		chromedp.KeyEvent(kb.Enter),
	)
	var id fetch.RequestID
	select {
	case id = <-paused:
	case <-time.After(10 * time.Second):
		t.Fatal("the repeated search sent no request")
	}
	if got := eval[string](t, ctx, announced); got != "" {
		t.Errorf("announcer = %q while the lookup is in flight, want the previous text hidden", got)
	}
	if role := eval[string](t, ctx, `document.getElementById("announcer").getAttribute("role")`); role != "status" {
		t.Errorf("announcer role = %q, want the live region to stay in place", role)
	}
	run(t, ctx, fetch.ContinueRequest(id))
	waitFor(t, ctx, `!document.querySelector("form button").disabled`)
	if got := eval[string](t, ctx, announced); got != notFound {
		t.Errorf("announcer = %q after the repeated search, want %q again", got, notFound)
	}
}
