package weather_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dev-labs-ai/weather-panel/internal/openmeteo"
	"github.com/dev-labs-ai/weather-panel/internal/weather"
)

var (
	recife = openmeteo.Place{
		ID: 3390760, Name: "Recife", Admin1: "Pernambuco", Country: "Brasil", Latitude: -8.05389, Longitude: -34.88111,
	}
	recifeNow = openmeteo.Current{
		TemperatureC: 25.3, ApparentTemperatureC: 29.1, RelativeHumidityPct: 77, WindSpeedKmh: 3.9, WeatherCode: 3,
	}
	recifeReport = weather.Report{
		Location: weather.Location{
			Name: "Recife", Admin1: "Pernambuco", Country: "Brasil", Latitude: -8.05389, Longitude: -34.88111,
		},
		Conditions: weather.Conditions{
			TemperatureC: 25.3, ApparentTemperatureC: 29.1, RelativeHumidityPct: 77, WindSpeedKmh: 3.9, WeatherCode: 3,
		},
	}
)

// fakeProvider answers like Recife unless a test replaces search or current, and records every call.
type fakeProvider struct {
	search  func(ctx context.Context, name string) (openmeteo.Place, error)
	current func(ctx context.Context, lat, lon float64) (openmeteo.Current, error)

	mu        sync.Mutex
	names     []string
	coords    [][2]float64
	deadlines []time.Time
}

func (p *fakeProvider) Search(ctx context.Context, name string) (openmeteo.Place, error) {
	p.record(ctx, func() { p.names = append(p.names, name) })
	if p.search != nil {
		return p.search(ctx, name)
	}
	return recife, nil
}

func (p *fakeProvider) Current(ctx context.Context, lat, lon float64) (openmeteo.Current, error) {
	p.record(ctx, func() { p.coords = append(p.coords, [2]float64{lat, lon}) })
	if p.current != nil {
		return p.current(ctx, lat, lon)
	}
	return recifeNow, nil
}

func (p *fakeProvider) record(ctx context.Context, add func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	add()
	deadline, _ := ctx.Deadline()
	p.deadlines = append(p.deadlines, deadline)
}

// blockUntilDone behaves like the real client on a hung connection: it gives up when the deadline passes.
func blockUntilDone[T any](ctx context.Context) (T, error) {
	<-ctx.Done()
	var zero T
	return zero, fmt.Errorf("fake: %w: %w", openmeteo.ErrTimeout, ctx.Err())
}

func newService(provider weather.Provider, cache weather.Cache, timeout time.Duration) (*weather.Service, *bytes.Buffer) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return weather.NewService(provider, cache, timeout, logger), &logs
}

func TestLookupSuccess(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{}
	svc, _ := newService(provider, weather.NopCache{}, time.Second)

	got, err := svc.Lookup(t.Context(), "  recife  ")
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if got != recifeReport {
		t.Errorf("Lookup() = %+v, want %+v", got, recifeReport)
	}
	if len(provider.names) != 1 || provider.names[0] != "recife" {
		t.Errorf("Search() names = %q, want [recife]", provider.names)
	}
	if len(provider.coords) != 1 || provider.coords[0] != [2]float64{recife.Latitude, recife.Longitude} {
		t.Errorf("Current() coordinates = %v, want the resolved location's", provider.coords)
	}
}

func TestLookupSendsTheNormalizedName(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{}
	svc, _ := newService(provider, weather.NopCache{}, time.Second)
	if _, err := svc.Lookup(t.Context(), " Sa\u0303o \t  Paulo "); err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if len(provider.names) != 1 || provider.names[0] != "São Paulo" {
		t.Errorf("Search() names = %q, want [São Paulo]", provider.names)
	}
}

func TestLookupFailures(t *testing.T) {
	t.Parallel()

	failWith := func(err error) func(context.Context, string) (openmeteo.Place, error) {
		return func(context.Context, string) (openmeteo.Place, error) { return openmeteo.Place{}, err }
	}
	tests := []struct {
		name          string
		input         string
		provider      *fakeProvider
		want          error
		wantSearches  int
		wantCurrents  int
		wantOutcome   string
		notWantErrors []error
	}{
		{
			name:     "not found",
			input:    "Xyzzyqqq",
			provider: &fakeProvider{search: failWith(fmt.Errorf("geocoding: %w", openmeteo.ErrNotFound))},
			want:     weather.ErrNotFound, wantSearches: 1, wantOutcome: "not_found",
			notWantErrors: []error{weather.ErrUpstream, weather.ErrUpstreamTimeout},
		},
		{
			name:     "geocoding upstream error",
			input:    "Recife",
			provider: &fakeProvider{search: failWith(fmt.Errorf("geocoding: %w: status 500", openmeteo.ErrUpstream))},
			want:     weather.ErrUpstream, wantSearches: 1, wantOutcome: "upstream_error",
			notWantErrors: []error{weather.ErrNotFound, weather.ErrUpstreamTimeout},
		},
		{
			name:     "geocoding timeout",
			input:    "Recife",
			provider: &fakeProvider{search: failWith(fmt.Errorf("geocoding: %w", openmeteo.ErrTimeout))},
			want:     weather.ErrUpstreamTimeout, wantSearches: 1, wantOutcome: "upstream_timeout",
			notWantErrors: []error{weather.ErrNotFound, weather.ErrUpstream},
		},
		{
			name:  "forecast upstream error",
			input: "Recife",
			provider: &fakeProvider{current: func(context.Context, float64, float64) (openmeteo.Current, error) {
				return openmeteo.Current{}, fmt.Errorf("forecast: %w: response has no current", openmeteo.ErrUpstream)
			}},
			want: weather.ErrUpstream, wantSearches: 1, wantCurrents: 1, wantOutcome: "upstream_error",
			notWantErrors: []error{weather.ErrNotFound, weather.ErrUpstreamTimeout},
		},
		{
			name:  "forecast timeout",
			input: "Recife",
			provider: &fakeProvider{current: func(context.Context, float64, float64) (openmeteo.Current, error) {
				return openmeteo.Current{}, fmt.Errorf("forecast: %w", openmeteo.ErrTimeout)
			}},
			want: weather.ErrUpstreamTimeout, wantSearches: 1, wantCurrents: 1, wantOutcome: "upstream_timeout",
			notWantErrors: []error{weather.ErrNotFound, weather.ErrUpstream},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc, logs := newService(tt.provider, weather.NopCache{}, time.Second)
			got, err := svc.Lookup(t.Context(), tt.input)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Lookup() error = %v, want %v", err, tt.want)
			}
			for _, other := range tt.notWantErrors {
				if errors.Is(err, other) {
					t.Errorf("Lookup() error = %v, want it not to match %v", err, other)
				}
			}
			if got != (weather.Report{}) {
				t.Errorf("Lookup() = %+v, want the zero Report on error", got)
			}
			// Nothing is retried: each upstream call happens at most once.
			if len(tt.provider.names) != tt.wantSearches || len(tt.provider.coords) != tt.wantCurrents {
				t.Errorf("calls: %d searches and %d forecasts, want %d and %d",
					len(tt.provider.names), len(tt.provider.coords), tt.wantSearches, tt.wantCurrents)
			}
			if !strings.Contains(logs.String(), `"outcome":"`+tt.wantOutcome+`"`) {
				t.Errorf("logs do not record outcome %q:\n%s", tt.wantOutcome, logs)
			}
		})
	}
}

func TestLookupInvalidInputSkipsTheProvider(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{}
	svc, logs := newService(provider, weather.NopCache{}, time.Second)

	_, err := svc.Lookup(t.Context(), " a ")
	var verr weather.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("Lookup() error = %v, want a ValidationError", err)
	}
	if len(provider.names)+len(provider.coords) != 0 {
		t.Errorf("provider was called %d times, want 0", len(provider.names)+len(provider.coords))
	}
	if !strings.Contains(logs.String(), `"outcome":"invalid_input"`) {
		t.Errorf("logs do not record the invalid input:\n%s", logs)
	}
}

func TestLookupCutsOffASlowUpstream(t *testing.T) {
	t.Parallel()

	const timeout = 100 * time.Millisecond
	tests := []struct {
		name     string
		provider *fakeProvider
	}{
		{"slow geocoding", &fakeProvider{search: func(ctx context.Context, _ string) (openmeteo.Place, error) {
			return blockUntilDone[openmeteo.Place](ctx)
		}}},
		{"slow forecast", &fakeProvider{current: func(ctx context.Context, _, _ float64) (openmeteo.Current, error) {
			return blockUntilDone[openmeteo.Current](ctx)
		}}},
		{"each call slow but within the deadline on its own", &fakeProvider{
			search: func(ctx context.Context, _ string) (openmeteo.Place, error) {
				select {
				case <-time.After(3 * timeout / 4):
					return recife, nil
				case <-ctx.Done():
					return blockUntilDone[openmeteo.Place](ctx)
				}
			},
			current: func(ctx context.Context, _, _ float64) (openmeteo.Current, error) {
				select {
				case <-time.After(3 * timeout / 4):
					return recifeNow, nil
				case <-ctx.Done():
					return blockUntilDone[openmeteo.Current](ctx)
				}
			},
		}},
		{"provider returns the bare context error", &fakeProvider{
			search: func(ctx context.Context, _ string) (openmeteo.Place, error) {
				<-ctx.Done()
				return openmeteo.Place{}, ctx.Err()
			},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc, _ := newService(tt.provider, weather.NopCache{}, timeout)
			start := time.Now()
			_, err := svc.Lookup(t.Context(), "Recife")
			elapsed := time.Since(start)

			if !errors.Is(err, weather.ErrUpstreamTimeout) {
				t.Fatalf("Lookup() error = %v, want ErrUpstreamTimeout", err)
			}
			if elapsed > timeout+200*time.Millisecond {
				t.Errorf("Lookup() took %v, want it cut off near the %v deadline", elapsed, timeout)
			}
		})
	}
}

func TestLookupSharesOneDeadline(t *testing.T) {
	t.Parallel()

	const timeout = 2 * time.Second
	provider := &fakeProvider{}
	svc, _ := newService(provider, weather.NopCache{}, timeout)

	start := time.Now()
	if _, err := svc.Lookup(t.Context(), "Recife"); err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	end := time.Now()
	if len(provider.deadlines) != 2 {
		t.Fatalf("provider saw %d calls, want 2", len(provider.deadlines))
	}
	search, current := provider.deadlines[0], provider.deadlines[1]
	if search.IsZero() {
		t.Fatal("Search() ran without a deadline")
	}
	if !search.Equal(current) {
		t.Errorf("Search() deadline %v, Current() deadline %v, want one shared deadline", search, current)
	}
	if search.Before(start.Add(timeout)) || search.After(end.Add(timeout)) {
		t.Errorf("deadline is %v after the lookup started, want %v", search.Sub(start), timeout)
	}
}

func TestLookupLogsNeverNameTheCityOrLocation(t *testing.T) {
	t.Parallel()

	for name, provider := range map[string]*fakeProvider{
		"success": {},
		"not found": {search: func(context.Context, string) (openmeteo.Place, error) {
			return openmeteo.Place{}, openmeteo.ErrNotFound
		}},
		"upstream error": {current: func(context.Context, float64, float64) (openmeteo.Current, error) {
			return openmeteo.Current{}, fmt.Errorf("forecast: %w: status 502", openmeteo.ErrUpstream)
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			svc, logs := newService(provider, weather.NopCache{}, time.Second)
			_, _ = svc.Lookup(t.Context(), "Recife")

			line := logs.String()
			for _, leak := range []string{"Recife", "recife", "Pernambuco", "Brasil", "-8.05", "-34.88", "3390760"} {
				if strings.Contains(line, leak) {
					t.Errorf("log %q contains %q", line, leak)
				}
			}
			if !strings.Contains(line, `"msg":"lookup"`) || !strings.Contains(line, `"duration_ms"`) {
				t.Errorf("log %q does not record the lookup", line)
			}
		})
	}
}

func TestLookupLogsUpstreamLatencies(t *testing.T) {
	t.Parallel()

	svc, logs := newService(&fakeProvider{}, weather.NopCache{}, time.Second)
	if _, err := svc.Lookup(t.Context(), "Recife"); err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	for _, want := range []string{
		`"geocoding_cache":"miss"`, `"weather_cache":"miss"`, `"geocoding_ms":`, `"forecast_ms":`, `"outcome":"success"`,
	} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log %q does not contain %s", logs, want)
		}
	}
}

// fakeCache is an in-memory Cache. When err is set, every call fails with it.
type fakeCache struct {
	mu         sync.Mutex
	places     map[string]*openmeteo.Place // nil value: cached "not found"
	conditions map[int64]openmeteo.Current
	err        error
	block      bool // every call waits for its context to end
}

func newFakeCache() *fakeCache {
	return &fakeCache{places: map[string]*openmeteo.Place{}, conditions: map[int64]openmeteo.Current{}}
}

func (c *fakeCache) fail(ctx context.Context) error {
	if c.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return c.err
}

func (c *fakeCache) Place(ctx context.Context, key string) (openmeteo.Place, bool, error) {
	if err := c.fail(ctx); err != nil {
		return openmeteo.Place{}, false, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.places[key]
	switch {
	case !ok:
		return openmeteo.Place{}, false, weather.ErrCacheMiss
	case p == nil:
		return openmeteo.Place{}, false, nil
	}
	return *p, true, nil
}

func (c *fakeCache) SavePlace(ctx context.Context, key string, place openmeteo.Place) error {
	if err := c.fail(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.places[key] = &place
	return nil
}

func (c *fakeCache) SaveNotFound(ctx context.Context, key string) error {
	if err := c.fail(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.places[key] = nil
	return nil
}

func (c *fakeCache) Conditions(ctx context.Context, locationID int64) (openmeteo.Current, error) {
	if err := c.fail(ctx); err != nil {
		return openmeteo.Current{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	cur, ok := c.conditions[locationID]
	if !ok {
		return openmeteo.Current{}, weather.ErrCacheMiss
	}
	return cur, nil
}

func (c *fakeCache) SaveConditions(ctx context.Context, locationID int64, current openmeteo.Current) error {
	if err := c.fail(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conditions[locationID] = current
	return nil
}

func TestLookupStoresAnswersAndServesThemFromCache(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{}
	cache := newFakeCache()
	svc, logs := newService(provider, cache, time.Second)

	for range 2 {
		got, err := svc.Lookup(t.Context(), "RECIFE")
		if err != nil {
			t.Fatalf("Lookup() error = %v", err)
		}
		if got != recifeReport {
			t.Errorf("Lookup() = %+v, want %+v", got, recifeReport)
		}
	}
	if len(provider.names) != 1 || len(provider.coords) != 1 {
		t.Errorf("provider saw %d searches and %d forecasts, want 1 of each", len(provider.names), len(provider.coords))
	}
	if p := cache.places["recife"]; p == nil || *p != recife {
		t.Errorf("cached place for %q = %v, want %+v", "recife", p, recife)
	}
	if !strings.Contains(logs.String(), `"geocoding_cache":"hit"`) || !strings.Contains(logs.String(), `"weather_cache":"hit"`) {
		t.Errorf("logs do not record the cache hits:\n%s", logs)
	}
}

func TestLookupCachesNotFound(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{search: func(context.Context, string) (openmeteo.Place, error) {
		return openmeteo.Place{}, openmeteo.ErrNotFound
	}}
	cache := newFakeCache()
	svc, _ := newService(provider, cache, time.Second)

	for range 2 {
		if _, err := svc.Lookup(t.Context(), "Xyzzyqqq"); !errors.Is(err, weather.ErrNotFound) {
			t.Fatalf("Lookup() error = %v, want ErrNotFound", err)
		}
	}
	if len(provider.names) != 1 {
		t.Errorf("provider saw %d searches, want 1", len(provider.names))
	}
}

func TestLookupNeverCachesUpstreamErrors(t *testing.T) {
	t.Parallel()

	for name, provider := range map[string]*fakeProvider{
		"geocoding": {search: func(context.Context, string) (openmeteo.Place, error) {
			return openmeteo.Place{}, openmeteo.ErrUpstream
		}},
		"geocoding timeout": {search: func(context.Context, string) (openmeteo.Place, error) {
			return openmeteo.Place{}, openmeteo.ErrTimeout
		}},
		"forecast": {current: func(context.Context, float64, float64) (openmeteo.Current, error) {
			return openmeteo.Current{}, openmeteo.ErrUpstream
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cache := newFakeCache()
			svc, _ := newService(provider, cache, time.Second)
			if _, err := svc.Lookup(t.Context(), "Recife"); err == nil {
				t.Fatal("Lookup() error = nil, want an upstream error")
			}
			if _, ok := cache.places["recife"]; ok && name != "forecast" {
				t.Errorf("cache holds a geocoding answer after a geocoding failure")
			}
			if len(cache.conditions) != 0 {
				t.Errorf("cache holds conditions after a failure: %v", cache.conditions)
			}
		})
	}
}

func TestLookupSucceedsWhenTheCacheFails(t *testing.T) {
	t.Parallel()

	for name, cache := range map[string]*fakeCache{
		"cache errors":         {err: errors.New("connection refused")},
		"cache stops replying": {block: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			provider := &fakeProvider{}
			svc, logs := newService(provider, cache, 2500*time.Millisecond)
			got, err := svc.Lookup(t.Context(), "Recife")
			if err != nil {
				t.Fatalf("Lookup() error = %v", err)
			}
			if got != recifeReport {
				t.Errorf("Lookup() = %+v, want %+v", got, recifeReport)
			}
			if !strings.Contains(logs.String(), `"msg":"cache unavailable"`) {
				t.Errorf("logs do not record the cache failure:\n%s", logs)
			}
			if !strings.Contains(logs.String(), `"geocoding_cache":"error"`) {
				t.Errorf("lookup log does not record the cache error:\n%s", logs)
			}
		})
	}
}
