package view

import (
	"math"
	"testing"

	"github.com/dev-labs-ai/weather-panel/internal/weather"
)

func TestFormatTemperature(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in   float64
		want string
	}{
		{23, "23 °C"},
		{23.4, "23 °C"},
		{23.5, "24 °C"},
		{22.5, "23 °C"},
		{0, "0 °C"},
		{math.Copysign(0, -1), "0 °C"},
		{-0.4, "0 °C"},
		{0.4, "0 °C"},
		{-0.5, "-1 °C"},
		{-0.6, "-1 °C"},
		{-12.3, "-12 °C"},
		{-12.5, "-13 °C"},
		{41.9, "42 °C"},
	} {
		if got := formatTemperature(tt.in); got != tt.want {
			t.Errorf("formatTemperature(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatHumidity(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in   float64
		want string
	}{
		{65, "65%"},
		{64.5, "65%"},
		{64.4, "64%"},
		{0, "0%"},
		{100, "100%"},
	} {
		if got := formatHumidity(tt.in); got != tt.want {
			t.Errorf("formatHumidity(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatWindSpeed(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		in   float64
		want string
	}{
		{12, "12 km/h"},
		{11.5, "12 km/h"},
		{11.4, "11 km/h"},
		{0, "0 km/h"},
		{0.4, "0 km/h"},
		{104.7, "105 km/h"},
	} {
		if got := formatWindSpeed(tt.in); got != tt.want {
			t.Errorf("formatWindSpeed(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatLocation(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		in   weather.Location
		want string
	}{
		{"every part", weather.Location{Name: "Recife", Admin1: "Pernambuco", Country: "Brasil"}, "Recife, Pernambuco, Brasil"},
		{"no admin1", weather.Location{Name: "Mônaco", Country: "Mônaco"}, "Mônaco, Mônaco"},
		{"no country", weather.Location{Name: "Recife", Admin1: "Pernambuco"}, "Recife, Pernambuco"},
		{"name only", weather.Location{Name: "Atlântida"}, "Atlântida"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := formatLocation(tt.in); got != tt.want {
				t.Errorf("formatLocation(%+v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
