package view

import (
	"math"
	"strconv"
	"strings"

	"github.com/dev-labs-ai/weather-panel/internal/weather"
)

// nbsp joins a number to its unit so that a narrow screen never wraps "23" and "°C" onto separate lines.
const nbsp = " "

// formatTemperature rounds to whole degrees Celsius: "23 °C".
func formatTemperature(c float64) string { return roundToString(c) + nbsp + "°C" }

// formatHumidity rounds to a whole percentage: "65%".
func formatHumidity(pct float64) string { return roundToString(pct) + "%" }

// formatWindSpeed rounds to whole kilometers per hour: "12 km/h".
func formatWindSpeed(kmh float64) string { return roundToString(kmh) + nbsp + "km/h" }

// roundToString rounds half away from zero. Converting to int drops the sign of a negative zero, so -0.4 °C reads
// "0 °C", never "-0 °C".
func roundToString(v float64) string { return strconv.Itoa(int(math.Round(v))) }

// formatLocation lists the resolved place as "Name, Admin1, Country", skipping the parts the provider did not return.
func formatLocation(l weather.Location) string {
	parts := make([]string, 0, 3)
	for _, p := range []string{l.Name, l.Admin1, l.Country} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}
