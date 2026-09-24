package cache

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dev-labs-ai/weather-panel/internal/openmeteo"
	"github.com/dev-labs-ai/weather-panel/internal/store"
	"github.com/dev-labs-ai/weather-panel/internal/weather"
)

// Entry lifetimes.
const (
	// PlaceTTL keeps a resolved location; place names and coordinates rarely change.
	PlaceTTL = 30 * 24 * time.Hour
	// NotFoundTTL keeps a "not found" answer short enough for new places to appear.
	NotFoundTTL = 24 * time.Hour
	// ConditionsTTL stays below the 15-minute interval at which Open-Meteo updates current conditions.
	ConditionsTTL = 10 * time.Minute
)

// Cache is the PostgreSQL implementation of weather.Cache. It stores nothing tied to a user, a session, or an IP
// address, and every entry expires on its own.
type Cache struct {
	queries *store.Queries
	now     func() time.Time
}

var _ weather.Cache = (*Cache)(nil)

// New returns a cache on db, which is normally a *pgxpool.Pool.
func New(db store.DBTX) *Cache {
	return NewWithClock(db, time.Now)
}

// NewWithClock returns a cache that reads the time from now. The application's clock alone decides expiry: every
// query receives the current time instead of calling the database's now().
func NewWithClock(db store.DBTX, now func() time.Time) *Cache {
	return &Cache{queries: store.New(db), now: now}
}

// Place returns the live geocoding answer for key, or weather.ErrCacheMiss.
func (c *Cache) Place(ctx context.Context, key string) (openmeteo.Place, bool, error) {
	row, err := c.queries.GetGeocoding(ctx, store.GetGeocodingParams{Query: key, Now: c.now()})
	if errors.Is(err, pgx.ErrNoRows) {
		return openmeteo.Place{}, false, weather.ErrCacheMiss
	}
	if err != nil {
		return openmeteo.Place{}, false, fmt.Errorf("read geocoding cache: %w", err)
	}
	if !row.Found {
		return openmeteo.Place{}, false, nil
	}
	if row.LocationID == nil || row.Name == nil || row.Latitude == nil || row.Longitude == nil {
		return openmeteo.Place{}, false, errors.New("read geocoding cache: found entry is missing required columns")
	}
	return openmeteo.Place{
		ID:        *row.LocationID,
		Name:      *row.Name,
		Admin1:    valueOrEmpty(row.Admin1),
		Country:   valueOrEmpty(row.Country),
		Latitude:  *row.Latitude,
		Longitude: *row.Longitude,
	}, true, nil
}

// SavePlace stores a resolved location for key for PlaceTTL.
func (c *Cache) SavePlace(ctx context.Context, key string, place openmeteo.Place) error {
	err := c.queries.UpsertGeocoding(ctx, store.UpsertGeocodingParams{
		Query:      key,
		Found:      true,
		LocationID: &place.ID,
		Name:       &place.Name,
		Admin1:     nilIfEmpty(place.Admin1),
		Country:    nilIfEmpty(place.Country),
		Latitude:   &place.Latitude,
		Longitude:  &place.Longitude,
		ExpiresAt:  c.now().Add(PlaceTTL),
	})
	if err != nil {
		return fmt.Errorf("write geocoding cache: %w", err)
	}
	return nil
}

// SaveNotFound stores for NotFoundTTL that no location matches key.
func (c *Cache) SaveNotFound(ctx context.Context, key string) error {
	err := c.queries.UpsertGeocoding(ctx, store.UpsertGeocodingParams{
		Query:     key,
		Found:     false,
		ExpiresAt: c.now().Add(NotFoundTTL),
	})
	if err != nil {
		return fmt.Errorf("write geocoding cache: %w", err)
	}
	return nil
}

// Conditions returns the live conditions for an Open-Meteo location id, or weather.ErrCacheMiss.
func (c *Cache) Conditions(ctx context.Context, locationID int64) (openmeteo.Current, error) {
	row, err := c.queries.GetWeather(ctx, store.GetWeatherParams{LocationID: locationID, Now: c.now()})
	if errors.Is(err, pgx.ErrNoRows) {
		return openmeteo.Current{}, weather.ErrCacheMiss
	}
	if err != nil {
		return openmeteo.Current{}, fmt.Errorf("read weather cache: %w", err)
	}
	return openmeteo.Current{
		TemperatureC:         row.TemperatureC,
		ApparentTemperatureC: row.ApparentTemperatureC,
		RelativeHumidityPct:  row.RelativeHumidityPct,
		WindSpeedKmh:         row.WindSpeedKmh,
		WeatherCode:          int(row.WeatherCode),
	}, nil
}

// SaveConditions stores the conditions for an Open-Meteo location id for ConditionsTTL.
func (c *Cache) SaveConditions(ctx context.Context, locationID int64, current openmeteo.Current) error {
	err := c.queries.UpsertWeather(ctx, store.UpsertWeatherParams{
		LocationID:           locationID,
		TemperatureC:         current.TemperatureC,
		ApparentTemperatureC: current.ApparentTemperatureC,
		RelativeHumidityPct:  current.RelativeHumidityPct,
		WindSpeedKmh:         current.WindSpeedKmh,
		WeatherCode:          int32(current.WeatherCode),
		ExpiresAt:            c.now().Add(ConditionsTTL),
	})
	if err != nil {
		return fmt.Errorf("write weather cache: %w", err)
	}
	return nil
}

// DeleteExpired removes every expired entry from both tables and returns how many it removed from each.
func (c *Cache) DeleteExpired(ctx context.Context) (geocoding, conditions int64, err error) {
	row, err := c.queries.DeleteExpired(ctx, c.now())
	if err != nil {
		return 0, 0, fmt.Errorf("delete expired cache entries: %w", err)
	}
	return row.GeocodingDeleted, row.WeatherDeleted, nil
}

// RunCleanup deletes expired entries now and then every interval until ctx is canceled. Reads already ignore expired
// entries; the cleanup only keeps the tables from growing. A failed run is logged and retried at the next interval.
func (c *Cache) RunCleanup(ctx context.Context, interval time.Duration, logger *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		geocoding, conditions, err := c.DeleteExpired(ctx)
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			logger.LogAttrs(ctx, slog.LevelWarn, "cache cleanup failed", slog.String("error", err.Error()))
		default:
			logger.LogAttrs(ctx, slog.LevelInfo, "cache cleanup",
				slog.Int64("geocoding_deleted", geocoding), slog.Int64("weather_deleted", conditions))
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func valueOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
