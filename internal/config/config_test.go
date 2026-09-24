package config_test

import (
	"log/slog"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/dev-labs-ai/weather-panel/internal/config"
)

const databaseURL = "postgres://weather:secret@localhost:5432/weather"

func env(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

func TestLoadDefaults(t *testing.T) {
	t.Parallel()

	for name, vars := range map[string]map[string]string{
		"unset": {"DATABASE_URL": databaseURL},
		"empty": {
			"DATABASE_URL":             databaseURL,
			"PORT":                     "",
			"LOG_LEVEL":                "",
			"LOOKUP_TIMEOUT":           "",
			"OPEN_METEO_GEOCODING_URL": "",
			"OPEN_METEO_FORECAST_URL":  "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := config.Load(env(vars))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			want := config.Config{
				Port:          8080,
				DatabaseURL:   databaseURL,
				LogLevel:      slog.LevelInfo,
				LookupTimeout: 2500 * time.Millisecond,
				GeocodingURL:  "https://geocoding-api.open-meteo.com/v1/search",
				ForecastURL:   "https://api.open-meteo.com/v1/forecast",
			}
			if got != want {
				t.Errorf("Load() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Parallel()

	got, err := config.Load(env(map[string]string{
		"PORT":                     "9090",
		"DATABASE_URL":             databaseURL,
		"LOG_LEVEL":                "debug",
		"LOOKUP_TIMEOUT":           "1500ms",
		"OPEN_METEO_GEOCODING_URL": "http://127.0.0.1:4001/v1/search",
		"OPEN_METEO_FORECAST_URL":  "http://127.0.0.1:4002/v1/forecast",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := config.Config{
		Port:          9090,
		DatabaseURL:   databaseURL,
		LogLevel:      slog.LevelDebug,
		LookupTimeout: 1500 * time.Millisecond,
		GeocodingURL:  "http://127.0.0.1:4001/v1/search",
		ForecastURL:   "http://127.0.0.1:4002/v1/forecast",
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadLogLevels(t *testing.T) {
	t.Parallel()

	for value, want := range map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
		"WARN":  slog.LevelWarn,
	} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			cfg, err := config.Load(env(map[string]string{"DATABASE_URL": databaseURL, "LOG_LEVEL": value}))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.LogLevel != want {
				t.Errorf("LogLevel = %v, want %v", cfg.LogLevel, want)
			}
		})
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		vars    map[string]string
		wantVar string
	}{
		{"missing database URL", map[string]string{"DATABASE_URL": ""}, "DATABASE_URL"},
		{"port not a number", map[string]string{"PORT": "abc"}, "PORT"},
		{"port zero", map[string]string{"PORT": "0"}, "PORT"},
		{"port out of range", map[string]string{"PORT": "65536"}, "PORT"},
		{"unknown log level", map[string]string{"LOG_LEVEL": "verbose"}, "LOG_LEVEL"},
		{"timeout without unit", map[string]string{"LOOKUP_TIMEOUT": "2500"}, "LOOKUP_TIMEOUT"},
		{"timeout zero", map[string]string{"LOOKUP_TIMEOUT": "0s"}, "LOOKUP_TIMEOUT"},
		{"timeout negative", map[string]string{"LOOKUP_TIMEOUT": "-1s"}, "LOOKUP_TIMEOUT"},
		{"geocoding URL relative", map[string]string{"OPEN_METEO_GEOCODING_URL": "/v1/search"}, "OPEN_METEO_GEOCODING_URL"},
		{"geocoding URL unparsable", map[string]string{"OPEN_METEO_GEOCODING_URL": "http://[::1"}, "OPEN_METEO_GEOCODING_URL"},
		{"forecast URL wrong scheme", map[string]string{"OPEN_METEO_FORECAST_URL": "ftp://api.open-meteo.com/v1/forecast"}, "OPEN_METEO_FORECAST_URL"},
		{"forecast URL without host", map[string]string{"OPEN_METEO_FORECAST_URL": "https:///v1/forecast"}, "OPEN_METEO_FORECAST_URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			vars := map[string]string{"DATABASE_URL": databaseURL}
			maps.Copy(vars, tt.vars)

			cfg, err := config.Load(env(vars))
			if err == nil {
				t.Fatalf("Load() = %+v, want an error", cfg)
			}
			if !strings.Contains(err.Error(), tt.wantVar) {
				t.Errorf("Load() error = %q, want it to name %s", err, tt.wantVar)
			}
			if cfg != (config.Config{}) {
				t.Errorf("Load() = %+v, want the zero Config on error", cfg)
			}
		})
	}
}

func TestLoadReportsEveryInvalidValue(t *testing.T) {
	t.Parallel()

	_, err := config.Load(env(map[string]string{"PORT": "abc", "LOG_LEVEL": "verbose"}))
	if err == nil {
		t.Fatal("Load() error = nil, want an error")
	}
	for _, name := range []string{"PORT", "DATABASE_URL", "LOG_LEVEL"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("Load() error = %q, want it to name %s", err, name)
		}
	}
}
