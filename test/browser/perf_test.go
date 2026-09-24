//go:build browser && perf

package browser_test

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/jackc/pgx/v5"
)

// perfCities is a representative set of searches: common names, accented names, and names shared by several places.
// Every entry is distinct, so each lookup misses the cache and calls both Open-Meteo APIs.
var perfCities = []struct{ kind, name string }{
	{"common", "São Paulo"}, {"common", "Rio de Janeiro"}, {"common", "Recife"}, {"common", "Salvador"},
	{"common", "Fortaleza"}, {"common", "Belo Horizonte"}, {"common", "Manaus"}, {"common", "Curitiba"},
	{"common", "Porto Alegre"}, {"common", "Brasília"}, {"common", "Lisboa"}, {"common", "Buenos Aires"},
	{"common", "Tokyo"}, {"common", "London"},
	{"accented", "São Luís"}, {"accented", "Florianópolis"}, {"accented", "Maceió"}, {"accented", "Cuiabá"},
	{"accented", "Goiânia"}, {"accented", "Jundiaí"}, {"accented", "Ribeirão Preto"}, {"accented", "Niterói"},
	{"accented", "Uberlândia"}, {"accented", "Ilhéus"}, {"accented", "Bogotá"}, {"accented", "Zürich"},
	{"accented", "Kraków"}, {"accented", "Reykjavík"},
	{"ambiguous", "Santa Maria"}, {"ambiguous", "Springfield"}, {"ambiguous", "Paris"}, {"ambiguous", "San José"},
	{"ambiguous", "Santa Cruz"}, {"ambiguous", "Victoria"}, {"ambiguous", "Valencia"}, {"ambiguous", "Cambridge"},
	{"ambiguous", "Boa Vista"}, {"ambiguous", "Alexandria"}, {"ambiguous", "Portland"}, {"ambiguous", "Córdoba"},
}

// timingHooks records, inside the page, when the form is submitted and when the swapped response has been painted.
const timingHooks = `(() => {
	window.__perf = {};
	document.querySelector("form").addEventListener("submit", () => {
		window.__perf = {start: performance.now()};
	}, {capture: true});
	document.addEventListener("htmx:after:swap", () => {
		requestAnimationFrame(() => { window.__perf.end = performance.now(); });
	}, {capture: true});
})()`

// TestLookupLatency measures AC-09 from submission to displayed result against the real Open-Meteo, starting from
// an empty cache. PERF_INTERVAL spaces the lookups (default 3s, about 40 upstream calls a minute, far below the
// free tier's 600). PERF_BEFORE_EACH is a shell command run before every lookup; restarting the panel there
// (`docker compose restart app`) makes every lookup open new connections to Open-Meteo, as the first lookup after an
// idle period does. PERF_DATABASE_URL is the database the panel caches in.
func TestLookupLatency(t *testing.T) {
	interval := 3 * time.Second
	if v := os.Getenv("PERF_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatalf("PERF_INTERVAL: %v", err)
		}
		interval = d
	}
	dbURL := cmp.Or(os.Getenv("PERF_DATABASE_URL"), "postgres://weather:weather@localhost:5432/weather?sslmode=disable")
	emptyCache(t, dbURL)

	beforeEach := os.Getenv("PERF_BEFORE_EACH")

	ctx := newTabFor(t, 1280, 800, time.Duration(len(perfCities))*(interval+40*time.Second))
	open(t, ctx)
	run(t, ctx, chromedp.Evaluate(timingHooks, nil))
	// A first search right after the page loads is slowed by the new tab settling in; warm it up with input the
	// panel rejects without calling Open-Meteo.
	search(t, ctx, "a")

	type sample struct {
		kind, name, heading string
		ms                  float64
	}
	var samples []sample
	var last time.Time
	for _, city := range perfCities {
		if wait := interval - time.Since(last); !last.IsZero() && wait > 0 {
			time.Sleep(wait)
		}
		if beforeEach != "" {
			runBeforeEach(t, beforeEach)
		}
		last = time.Now()
		run(t, ctx,
			chromedp.Evaluate(`window.__perf = {}; document.getElementById("city").value = ""`, nil),
			chromedp.Focus("#city", chromedp.ByID),
			chromedp.KeyEvent(city.name),
			chromedp.KeyEvent(kb.Enter),
		)
		waitFor(t, ctx, `window.__perf.end !== undefined`)
		ms := eval[float64](t, ctx, `window.__perf.end - window.__perf.start`)
		heading := eval[string](t, ctx, `document.querySelector("#result h2").textContent.trim().replace(/\s+/g, " ")`)
		samples = append(samples, sample{city.kind, city.name, heading, ms})
		t.Logf("%-9s %-16s %7.0f ms  %s", city.kind, city.name, ms, heading)
	}

	var within, results int
	durations := make([]float64, 0, len(samples))
	for _, s := range samples {
		ok := strings.HasPrefix(s.heading, "Agora em")
		if ok {
			results++
		}
		if ok && s.ms <= 3000 {
			within++
		}
		durations = append(durations, s.ms)
	}
	slices.Sort(durations)
	share := float64(within) / float64(len(samples))
	t.Logf("interval %v, before each %q: %d lookups, %d results, %d results within 3 s (%.1f%%); p50 %.0f ms, p95 %.0f ms, max %.0f ms",
		interval, beforeEach, len(samples), results, within, 100*share, percentile(durations, 50), percentile(durations, 95),
		durations[len(durations)-1])
	if share < 0.95 {
		t.Errorf("%.1f%% of lookups displayed a result within 3 s, want at least 95%%", 100*share)
	}
}

// percentile uses the nearest-rank method on sorted values.
func percentile(sorted []float64, p float64) float64 {
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	return sorted[max(rank, 1)-1]
}

// runBeforeEach runs the command and waits until the panel answers its health check again.
func runBeforeEach(t *testing.T, command string) {
	t.Helper()
	if out, err := exec.Command("sh", "-c", command).CombinedOutput(); err != nil {
		t.Fatalf("PERF_BEFORE_EACH: %v\n%s", err, out)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		resp, err := http.Get(baseURL + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("the panel did not come back after PERF_BEFORE_EACH: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func emptyCache(t *testing.T, dbURL string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to the panel's database to empty the cache: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, "TRUNCATE geocoding_cache, weather_cache"); err != nil {
		t.Fatalf("empty the cache: %v", err)
	}
	fmt.Fprintln(os.Stderr, "cache emptied")
}
