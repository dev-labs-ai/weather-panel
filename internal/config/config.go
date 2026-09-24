package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Config holds the service settings.
type Config struct {
	Port          int
	DatabaseURL   string
	LogLevel      slog.Level
	LookupTimeout time.Duration
	GeocodingURL  string
	ForecastURL   string
}

// Load reads the configuration through getenv, which is os.Getenv outside tests. An empty variable counts as unset
// and takes its default. Load reports every missing or malformed variable in one error, so they can all be fixed in
// a single pass.
func Load(getenv func(string) string) (Config, error) {
	var errs []error
	cfg := Config{
		Port:          optional(getenv, "PORT", 8080, parsePort, &errs),
		DatabaseURL:   required(getenv, "DATABASE_URL", &errs),
		LogLevel:      optional(getenv, "LOG_LEVEL", slog.LevelInfo, parseLogLevel, &errs),
		LookupTimeout: optional(getenv, "LOOKUP_TIMEOUT", 2500*time.Millisecond, parseTimeout, &errs),
		GeocodingURL: optional(getenv, "OPEN_METEO_GEOCODING_URL", "https://geocoding-api.open-meteo.com/v1/search",
			parseBaseURL, &errs),
		ForecastURL: optional(getenv, "OPEN_METEO_FORECAST_URL", "https://api.open-meteo.com/v1/forecast",
			parseBaseURL, &errs),
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, fmt.Errorf("invalid configuration: %w", err)
	}
	return cfg, nil
}

func required(getenv func(string) string, name string, errs *[]error) string {
	v := getenv(name)
	if v == "" {
		*errs = append(*errs, fmt.Errorf("%s is required", name))
	}
	return v
}

func optional[T any](getenv func(string) string, name string, def T, parse func(string) (T, error), errs *[]error) T {
	v := getenv(name)
	if v == "" {
		return def
	}
	parsed, err := parse(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s %w, got %q", name, err, v))
		return def
	}
	return parsed
}

func parsePort(v string) (int, error) {
	port, err := strconv.Atoi(v)
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New("must be a whole number from 1 to 65535")
	}
	return port, nil
}

func parseLogLevel(v string) (slog.Level, error) {
	switch strings.ToLower(v) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, errors.New("must be debug, info, warn, or error")
}

func parseTimeout(v string) (time.Duration, error) {
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, errors.New("must be a positive duration such as 2.5s")
	}
	return d, nil
}

func parseBaseURL(v string) (string, error) {
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("must be an absolute http or https URL")
	}
	return v, nil
}
