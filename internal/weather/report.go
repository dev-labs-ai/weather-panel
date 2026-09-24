package weather

// Location is the place the Geocoding API resolved a search to.
type Location struct {
	Name      string
	Admin1    string // optional: state, province, or other first-level division
	Country   string // optional
	Latitude  float64
	Longitude float64
}

// Conditions are the current weather values at a location, in metric units.
type Conditions struct {
	TemperatureC         float64
	ApparentTemperatureC float64
	RelativeHumidityPct  float64
	WindSpeedKmh         float64
	WeatherCode          int // WMO weather interpretation code
}

// Report is the outcome of a successful lookup.
type Report struct {
	Location   Location
	Conditions Conditions
}
