package weather

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/dev-labs-ai/weather-panel/internal/openmeteo"
)

// Lookup failures other than ValidationError. Handlers map each to a status code and a message.
var (
	ErrNotFound        = errors.New("city not found")
	ErrUpstream        = errors.New("weather provider unavailable")
	ErrUpstreamTimeout = errors.New("weather provider timed out")
)

// cacheTimeout bounds each cache call, so a database that stops answering costs a lookup a fraction of its deadline
// instead of all of it.
const cacheTimeout = 250 * time.Millisecond

// Provider is the Open-Meteo client.
type Provider interface {
	Search(ctx context.Context, name string) (openmeteo.Place, error)
	Current(ctx context.Context, latitude, longitude float64) (openmeteo.Current, error)
}

// ErrCacheMiss is what a Cache returns when it holds no live entry for the key.
var ErrCacheMiss = errors.New("cache miss")

// Cache keeps recent Open-Meteo answers. It is best-effort: the service logs any error other than ErrCacheMiss and
// carries on as if the entry were missing.
type Cache interface {
	// Place returns the geocoding answer stored for a Query.Key. found is false when the stored answer is that no
	// location matches.
	Place(ctx context.Context, key string) (place openmeteo.Place, found bool, err error)
	SavePlace(ctx context.Context, key string, place openmeteo.Place) error
	SaveNotFound(ctx context.Context, key string) error
	// Conditions returns the current conditions stored for an Open-Meteo location id.
	Conditions(ctx context.Context, locationID int64) (openmeteo.Current, error)
	SaveConditions(ctx context.Context, locationID int64, current openmeteo.Current) error
}

// NopCache stores nothing; every read is a miss.
type NopCache struct{}

func (NopCache) Place(context.Context, string) (openmeteo.Place, bool, error) {
	return openmeteo.Place{}, false, ErrCacheMiss
}
func (NopCache) SavePlace(context.Context, string, openmeteo.Place) error { return nil }
func (NopCache) SaveNotFound(context.Context, string) error               { return nil }
func (NopCache) Conditions(context.Context, int64) (openmeteo.Current, error) {
	return openmeteo.Current{}, ErrCacheMiss
}
func (NopCache) SaveConditions(context.Context, int64, openmeteo.Current) error { return nil }

// Service looks up the current weather for a city name.
type Service struct {
	provider Provider
	cache    Cache
	timeout  time.Duration
	logger   *slog.Logger
}

// NewService returns a service that answers from cache when it can and from provider otherwise, giving each lookup
// at most timeout.
func NewService(provider Provider, cache Cache, timeout time.Duration, logger *slog.Logger) *Service {
	return &Service{provider: provider, cache: cache, timeout: timeout, logger: logger}
}

// Lookup normalizes the input, resolves it to a location, and fetches the current conditions there. It returns a
// ValidationError for unusable input and an error wrapping ErrNotFound, ErrUpstream, or ErrUpstreamTimeout when the
// lookup fails. The whole lookup shares one deadline and nothing is retried; the user can retry from the panel.
func (s *Service) Lookup(ctx context.Context, input string) (Report, error) {
	start := time.Now()
	// The log records how the lookup went but never the city or the resolved location.
	attrs := make([]slog.Attr, 0, 8)
	report, err := s.lookup(ctx, input, &attrs)

	outcome, level := outcomeOf(err)
	attrs = append(attrs,
		slog.String("outcome", outcome),
		slog.Float64("duration_ms", millis(time.Since(start))),
	)
	if level > slog.LevelInfo {
		attrs = append(attrs, slog.String("error", err.Error()))
	}
	s.logger.LogAttrs(ctx, level, "lookup", attrs...)
	return report, err
}

func (s *Service) lookup(ctx context.Context, input string, attrs *[]slog.Attr) (Report, error) {
	q, err := ParseQuery(input)
	if err != nil {
		return Report{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	place, err := s.resolve(ctx, q, attrs)
	if err != nil {
		return Report{}, err
	}
	current, err := s.current(ctx, place, attrs)
	if err != nil {
		return Report{}, err
	}
	return Report{
		Location: Location{
			Name:      place.Name,
			Admin1:    place.Admin1,
			Country:   place.Country,
			Latitude:  place.Latitude,
			Longitude: place.Longitude,
		},
		Conditions: Conditions{
			TemperatureC:         current.TemperatureC,
			ApparentTemperatureC: current.ApparentTemperatureC,
			RelativeHumidityPct:  current.RelativeHumidityPct,
			WindSpeedKmh:         current.WindSpeedKmh,
			WeatherCode:          current.WeatherCode,
		},
	}, nil
}

// resolve returns the location for q from the cache or the Geocoding API, caching the API's answer, including a
// "not found" answer. Upstream errors are never cached.
func (s *Service) resolve(ctx context.Context, q Query, attrs *[]slog.Attr) (openmeteo.Place, error) {
	cacheCtx, cancel := context.WithTimeout(ctx, cacheTimeout)
	place, found, err := s.cache.Place(cacheCtx, q.Key)
	cancel()
	*attrs = append(*attrs, slog.String("geocoding_cache", s.cacheStatus(ctx, "read place", err)))
	if err == nil {
		if !found {
			return openmeteo.Place{}, ErrNotFound
		}
		return place, nil
	}

	start := time.Now()
	place, err = s.provider.Search(ctx, q.Text)
	*attrs = append(*attrs, slog.Float64("geocoding_ms", millis(time.Since(start))))
	switch {
	case errors.Is(err, openmeteo.ErrNotFound):
		s.store(ctx, "save not found", func(ctx context.Context) error { return s.cache.SaveNotFound(ctx, q.Key) })
		return openmeteo.Place{}, ErrNotFound
	case err != nil:
		return openmeteo.Place{}, upstreamError(ctx, err)
	}
	s.store(ctx, "save place", func(ctx context.Context) error { return s.cache.SavePlace(ctx, q.Key, place) })
	return place, nil
}

// current returns the conditions at place from the cache or the Forecast API, caching the API's answer.
func (s *Service) current(ctx context.Context, place openmeteo.Place, attrs *[]slog.Attr) (openmeteo.Current, error) {
	cacheCtx, cancel := context.WithTimeout(ctx, cacheTimeout)
	current, err := s.cache.Conditions(cacheCtx, place.ID)
	cancel()
	*attrs = append(*attrs, slog.String("weather_cache", s.cacheStatus(ctx, "read conditions", err)))
	if err == nil {
		return current, nil
	}

	start := time.Now()
	current, err = s.provider.Current(ctx, place.Latitude, place.Longitude)
	*attrs = append(*attrs, slog.Float64("forecast_ms", millis(time.Since(start))))
	if err != nil {
		return openmeteo.Current{}, upstreamError(ctx, err)
	}
	s.store(ctx, "save conditions", func(ctx context.Context) error {
		return s.cache.SaveConditions(ctx, place.ID, current)
	})
	return current, nil
}

// cacheStatus names a cache read's result for the lookup log, logging a cache failure on its own line.
func (s *Service) cacheStatus(ctx context.Context, op string, err error) string {
	switch {
	case err == nil:
		return "hit"
	case errors.Is(err, ErrCacheMiss):
		return "miss"
	}
	s.logCacheError(ctx, op, err)
	return "error"
}

// store runs a cache write, logging a failure instead of failing the lookup.
func (s *Service) store(ctx context.Context, op string, write func(context.Context) error) {
	cacheCtx, cancel := context.WithTimeout(ctx, cacheTimeout)
	defer cancel()
	if err := write(cacheCtx); err != nil {
		s.logCacheError(ctx, op, err)
	}
}

func (s *Service) logCacheError(ctx context.Context, op string, err error) {
	s.logger.LogAttrs(ctx, slog.LevelWarn, "cache unavailable", slog.String("op", op), slog.String("error", err.Error()))
}

// upstreamError wraps a provider failure in ErrUpstreamTimeout when the lookup ran out of time and in ErrUpstream
// otherwise.
func upstreamError(ctx context.Context, err error) error {
	if errors.Is(err, openmeteo.ErrTimeout) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrUpstreamTimeout, err)
	}
	return fmt.Errorf("%w: %w", ErrUpstream, err)
}

// outcomeOf names a lookup's result for the log and picks the log level: failures of the provider are warnings,
// everything the user caused is informational.
func outcomeOf(err error) (string, slog.Level) {
	var verr ValidationError
	switch {
	case err == nil:
		return "success", slog.LevelInfo
	case errors.As(err, &verr):
		return "invalid_input", slog.LevelInfo
	case errors.Is(err, ErrNotFound):
		return "not_found", slog.LevelInfo
	case errors.Is(err, ErrUpstreamTimeout):
		return "upstream_timeout", slog.LevelWarn
	default:
		return "upstream_error", slog.LevelWarn
	}
}

func millis(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
