package cache_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dev-labs-ai/weather-panel/db"
	"github.com/dev-labs-ai/weather-panel/internal/cache"
	"github.com/dev-labs-ai/weather-panel/internal/dbtest"
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
)

// clock is a settable time source shared by a cache and its test.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// newCache returns a cache on a fresh, migrated database, the pool behind it, and the cache's clock.
func newCache(t *testing.T) (*cache.Cache, *pgxpool.Pool, *clock) {
	t.Helper()
	url := dbtest.Fresh(t)
	if _, err := db.Migrate(url); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	pool, err := pgxpool.New(t.Context(), url)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	t.Cleanup(pool.Close)
	clk := &clock{now: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
	return cache.NewWithClock(pool, clk.Now), pool, clk
}

func assertPlace(t *testing.T, c *cache.Cache, key string, want openmeteo.Place) {
	t.Helper()
	got, found, err := c.Place(t.Context(), key)
	if err != nil {
		t.Fatalf("Place(%q) error = %v", key, err)
	}
	if !found || got != want {
		t.Errorf("Place(%q) = %+v, %v, want %+v, true", key, got, found, want)
	}
}

func assertPlaceMiss(t *testing.T, c *cache.Cache, key string) {
	t.Helper()
	if got, found, err := c.Place(t.Context(), key); !errors.Is(err, weather.ErrCacheMiss) {
		t.Errorf("Place(%q) = %+v, %v, %v, want ErrCacheMiss", key, got, found, err)
	}
}

func assertNotFound(t *testing.T, c *cache.Cache, key string) {
	t.Helper()
	got, found, err := c.Place(t.Context(), key)
	if err != nil || found || got != (openmeteo.Place{}) {
		t.Errorf("Place(%q) = %+v, %v, %v, want a cached not-found answer", key, got, found, err)
	}
}

func TestPlaceMissThenHit(t *testing.T) {
	t.Parallel()
	c, _, _ := newCache(t)

	assertPlaceMiss(t, c, "recife")
	if err := c.SavePlace(t.Context(), "recife", recife); err != nil {
		t.Fatalf("SavePlace() error = %v", err)
	}
	assertPlace(t, c, "recife", recife)
	assertPlaceMiss(t, c, "olinda")
}

func TestPlaceWithoutOptionalFields(t *testing.T) {
	t.Parallel()
	c, _, _ := newCache(t)

	monaco := openmeteo.Place{ID: 2993458, Name: "Mônaco", Latitude: 43.73333, Longitude: 7.41667}
	if err := c.SavePlace(t.Context(), "mônaco", monaco); err != nil {
		t.Fatalf("SavePlace() error = %v", err)
	}
	assertPlace(t, c, "mônaco", monaco)
}

func TestPlaceExpiresAfter30Days(t *testing.T) {
	t.Parallel()
	c, _, clk := newCache(t)

	if err := c.SavePlace(t.Context(), "recife", recife); err != nil {
		t.Fatalf("SavePlace() error = %v", err)
	}
	clk.Advance(30*24*time.Hour - time.Second)
	assertPlace(t, c, "recife", recife)
	clk.Advance(time.Second)
	assertPlaceMiss(t, c, "recife")
}

func TestNotFoundIsCachedForOneDay(t *testing.T) {
	t.Parallel()
	c, _, clk := newCache(t)

	if err := c.SaveNotFound(t.Context(), "xyzzyqqq"); err != nil {
		t.Fatalf("SaveNotFound() error = %v", err)
	}
	assertNotFound(t, c, "xyzzyqqq")
	clk.Advance(24*time.Hour - time.Second)
	assertNotFound(t, c, "xyzzyqqq")
	clk.Advance(time.Second)
	assertPlaceMiss(t, c, "xyzzyqqq")
}

func TestPlaceUpserts(t *testing.T) {
	t.Parallel()
	c, _, clk := newCache(t)
	ctx := t.Context()

	if err := c.SaveNotFound(ctx, "recife"); err != nil {
		t.Fatalf("SaveNotFound() error = %v", err)
	}
	if err := c.SavePlace(ctx, "recife", recife); err != nil {
		t.Fatalf("SavePlace() over a not-found entry error = %v", err)
	}
	assertPlace(t, c, "recife", recife)

	moved := recife
	moved.Admin1 = ""
	moved.Latitude = -8.1
	if err := c.SavePlace(ctx, "recife", moved); err != nil {
		t.Fatalf("SavePlace() over a found entry error = %v", err)
	}
	assertPlace(t, c, "recife", moved)

	// Saving again restarts the lifetime.
	clk.Advance(20 * 24 * time.Hour)
	if err := c.SavePlace(ctx, "recife", recife); err != nil {
		t.Fatalf("SavePlace() error = %v", err)
	}
	clk.Advance(20 * 24 * time.Hour)
	assertPlace(t, c, "recife", recife)

	if err := c.SaveNotFound(ctx, "recife"); err != nil {
		t.Fatalf("SaveNotFound() over a found entry error = %v", err)
	}
	assertNotFound(t, c, "recife")
}

func TestConditions(t *testing.T) {
	t.Parallel()
	c, _, clk := newCache(t)
	ctx := t.Context()

	if _, err := c.Conditions(ctx, recife.ID); !errors.Is(err, weather.ErrCacheMiss) {
		t.Fatalf("Conditions() on an empty cache error = %v, want ErrCacheMiss", err)
	}
	if err := c.SaveConditions(ctx, recife.ID, recifeNow); err != nil {
		t.Fatalf("SaveConditions() error = %v", err)
	}
	got, err := c.Conditions(ctx, recife.ID)
	if err != nil || got != recifeNow {
		t.Errorf("Conditions() = %+v, %v, want %+v", got, err, recifeNow)
	}

	later := openmeteo.Current{TemperatureC: -2.4, ApparentTemperatureC: -6, RelativeHumidityPct: 91, WindSpeedKmh: 30.2,
		WeatherCode: 71}
	clk.Advance(5 * time.Minute)
	if err := c.SaveConditions(ctx, recife.ID, later); err != nil {
		t.Fatalf("SaveConditions() over an entry error = %v", err)
	}
	clk.Advance(10*time.Minute - time.Second)
	if got, err := c.Conditions(ctx, recife.ID); err != nil || got != later {
		t.Errorf("Conditions() = %+v, %v, want the upserted %+v", got, err, later)
	}
	clk.Advance(time.Second)
	if _, err := c.Conditions(ctx, recife.ID); !errors.Is(err, weather.ErrCacheMiss) {
		t.Errorf("Conditions() after 10 minutes error = %v, want ErrCacheMiss", err)
	}
}

func TestDeleteExpired(t *testing.T) {
	t.Parallel()
	c, pool, clk := newCache(t)
	ctx := t.Context()

	// Expire in 10 minutes, 1 day, and 30 days.
	mustSave(t, c.SaveConditions(ctx, 1, recifeNow))
	mustSave(t, c.SaveNotFound(ctx, "xyzzyqqq"))
	mustSave(t, c.SavePlace(ctx, "recife", recife))
	clk.Advance(2 * 24 * time.Hour)
	mustSave(t, c.SaveConditions(ctx, 2, recifeNow))

	geocoding, conditions, err := c.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("DeleteExpired() error = %v", err)
	}
	if geocoding != 1 || conditions != 1 {
		t.Errorf("DeleteExpired() = %d, %d, want 1, 1", geocoding, conditions)
	}
	assertRows(t, pool, "SELECT query FROM geocoding_cache", "recife")
	assertRows(t, pool, "SELECT location_id::text FROM weather_cache", "2")

	if geocoding, conditions, err := c.DeleteExpired(ctx); err != nil || geocoding != 0 || conditions != 0 {
		t.Errorf("second DeleteExpired() = %d, %d, %v, want 0, 0, nil", geocoding, conditions, err)
	}
}

func TestRunCleanup(t *testing.T) {
	t.Parallel()
	c, pool, clk := newCache(t)

	mustSave(t, c.SaveConditions(t.Context(), 1, recifeNow))
	mustSave(t, c.SaveNotFound(t.Context(), "xyzzyqqq"))
	clk.Advance(2 * 24 * time.Hour)

	var logs syncBuffer
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		c.RunCleanup(ctx, time.Hour, slog.New(slog.NewJSONHandler(&logs, nil)))
		close(done)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), `"msg":"cache cleanup"`) {
		if time.Now().After(deadline) {
			t.Fatalf("cleanup did not run at start; logs:\n%s", logs.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunCleanup() did not return after cancellation")
	}
	assertRows(t, pool, "SELECT query FROM geocoding_cache")
	assertRows(t, pool, "SELECT location_id::text FROM weather_cache")
}

func TestLookupServedFromCache(t *testing.T) {
	t.Parallel()
	c, _, _ := newCache(t)

	provider := &countingProvider{}
	svc := weather.NewService(provider, c, time.Second, slog.New(slog.DiscardHandler))
	for _, input := range []string{"Recife", "  RECIFE "} {
		if _, err := svc.Lookup(t.Context(), input); err != nil {
			t.Fatalf("Lookup(%q) error = %v", input, err)
		}
	}
	if provider.searches != 1 || provider.forecasts != 1 {
		t.Errorf("provider saw %d searches and %d forecasts, want 1 of each", provider.searches, provider.forecasts)
	}
}

func TestDatabaseDown(t *testing.T) {
	t.Parallel()

	pool, err := pgxpool.New(t.Context(), dbtest.UnreachableURL)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	t.Cleanup(pool.Close)
	c := cache.New(pool)
	ctx := t.Context()

	if _, _, err := c.Place(ctx, "recife"); err == nil || errors.Is(err, weather.ErrCacheMiss) {
		t.Errorf("Place() error = %v, want a database error", err)
	}
	if _, err := c.Conditions(ctx, recife.ID); err == nil || errors.Is(err, weather.ErrCacheMiss) {
		t.Errorf("Conditions() error = %v, want a database error", err)
	}
	if err := c.SavePlace(ctx, "recife", recife); err == nil {
		t.Error("SavePlace() error = nil, want a database error")
	}
	if _, _, err := c.DeleteExpired(ctx); err == nil {
		t.Error("DeleteExpired() error = nil, want a database error")
	}
}

func TestLookupSucceedsWhileTheDatabaseIsDown(t *testing.T) {
	t.Parallel()

	pool, err := pgxpool.New(t.Context(), dbtest.UnreachableURL)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	t.Cleanup(pool.Close)

	var logs bytes.Buffer
	svc := weather.NewService(&countingProvider{}, cache.New(pool), 2500*time.Millisecond,
		slog.New(slog.NewJSONHandler(&logs, nil)))
	report, err := svc.Lookup(t.Context(), "Recife")
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if report.Location.Name != "Recife" || report.Conditions.TemperatureC != recifeNow.TemperatureC {
		t.Errorf("Lookup() = %+v, want Recife's report", report)
	}
	if !strings.Contains(logs.String(), `"msg":"cache unavailable"`) {
		t.Errorf("logs do not record the database failure:\n%s", logs.String())
	}
}

// countingProvider answers every lookup with Recife.
type countingProvider struct {
	mu                  sync.Mutex
	searches, forecasts int
}

func (p *countingProvider) Search(context.Context, string) (openmeteo.Place, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.searches++
	return recife, nil
}

func (p *countingProvider) Current(context.Context, float64, float64) (openmeteo.Current, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.forecasts++
	return recifeNow, nil
}

// syncBuffer lets a test read logs that another goroutine is still writing.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func mustSave(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("save: %v", err)
	}
}

// assertRows fails the test unless query returns exactly the want values, in any order.
func assertRows(t *testing.T, pool *pgxpool.Pool, query string, want ...string) {
	t.Helper()
	rows, err := pool.Query(t.Context(), query+" ORDER BY 1")
	if err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	var got []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%s = %v, want %v", query, got, want)
	}
}
