package openmeteo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
)

// maxBodyBytes caps how much of an upstream response the client reads.
const maxBodyBytes = 1 << 20

// currentVariables are the values requested from the Forecast API, in the order Open-Meteo documents them.
const currentVariables = "temperature_2m,apparent_temperature,relative_humidity_2m,weather_code,wind_speed_10m"

var (
	// ErrNotFound means the Geocoding API has no location for the name.
	ErrNotFound = errors.New("no location matches the name")
	// ErrUpstream means Open-Meteo could not be reached or sent a response the client cannot use.
	ErrUpstream = errors.New("open-meteo request failed")
	// ErrTimeout means the request's deadline passed before Open-Meteo answered.
	ErrTimeout = errors.New("open-meteo request timed out")
)

// Place is the first location the Geocoding API returns for a name.
type Place struct {
	ID        int64
	Name      string
	Admin1    string // empty when Open-Meteo omits it
	Country   string // empty when Open-Meteo omits it
	Latitude  float64
	Longitude float64
}

// Current holds the current weather values from the Forecast API, in °C, %, and km/h.
type Current struct {
	TemperatureC         float64
	ApparentTemperatureC float64
	RelativeHumidityPct  float64
	WindSpeedKmh         float64
	WeatherCode          int
}

// Client calls the Open-Meteo Geocoding and Forecast APIs. It is safe for concurrent use, and its requests share
// one connection pool so that lookups reuse TLS connections.
type Client struct {
	http         *http.Client
	geocodingURL string
	forecastURL  string
}

// NewClient returns a client for the Geocoding API at geocodingURL and the Forecast API at forecastURL. It sets no
// timeout of its own: every call is bounded by the deadline of its context.
func NewClient(geocodingURL, forecastURL string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 16
	return &Client{
		http:         &http.Client{Transport: transport},
		geocodingURL: geocodingURL,
		forecastURL:  forecastURL,
	}
}

// Search resolves name to the first matching location. It returns an error wrapping ErrNotFound when there is none,
// ErrTimeout when ctx's deadline passes, and ErrUpstream for any other failure.
func (c *Client) Search(ctx context.Context, name string) (Place, error) {
	var body struct {
		Results []struct {
			ID        *int64   `json:"id"`
			Name      *string  `json:"name"`
			Latitude  *float64 `json:"latitude"`
			Longitude *float64 `json:"longitude"`
			Admin1    string   `json:"admin1"`
			Country   string   `json:"country"`
		} `json:"results"`
	}
	err := c.get(ctx, "geocoding", c.geocodingURL, url.Values{
		"name":     {name},
		"count":    {"1"},
		"language": {"pt"},
		"format":   {"json"},
	}, &body)
	if err != nil {
		return Place{}, err
	}
	if len(body.Results) == 0 {
		return Place{}, fmt.Errorf("geocoding: %w", ErrNotFound)
	}

	r := body.Results[0]
	switch {
	case r.ID == nil:
		return Place{}, missingField("geocoding", "id")
	case r.Name == nil || *r.Name == "":
		return Place{}, missingField("geocoding", "name")
	case r.Latitude == nil:
		return Place{}, missingField("geocoding", "latitude")
	case r.Longitude == nil:
		return Place{}, missingField("geocoding", "longitude")
	}
	return Place{
		ID:        *r.ID,
		Name:      *r.Name,
		Admin1:    r.Admin1,
		Country:   r.Country,
		Latitude:  *r.Latitude,
		Longitude: *r.Longitude,
	}, nil
}

// Current fetches the current conditions at the coordinates. It returns an error wrapping ErrTimeout when ctx's
// deadline passes and ErrUpstream for any other failure.
func (c *Client) Current(ctx context.Context, latitude, longitude float64) (Current, error) {
	var body struct {
		Current *struct {
			Temperature         *float64 `json:"temperature_2m"`
			ApparentTemperature *float64 `json:"apparent_temperature"`
			RelativeHumidity    *float64 `json:"relative_humidity_2m"`
			WeatherCode         *int     `json:"weather_code"`
			WindSpeed           *float64 `json:"wind_speed_10m"`
		} `json:"current"`
	}
	// Celsius and km/h are the API defaults, but requesting them explicitly keeps a change of defaults from altering
	// what the panel shows.
	err := c.get(ctx, "forecast", c.forecastURL, url.Values{
		"latitude":         {strconv.FormatFloat(latitude, 'f', -1, 64)},
		"longitude":        {strconv.FormatFloat(longitude, 'f', -1, 64)},
		"current":          {currentVariables},
		"temperature_unit": {"celsius"},
		"wind_speed_unit":  {"kmh"},
	}, &body)
	if err != nil {
		return Current{}, err
	}

	cur := body.Current
	switch {
	case cur == nil:
		return Current{}, missingField("forecast", "current")
	case cur.Temperature == nil:
		return Current{}, missingField("forecast", "current.temperature_2m")
	case cur.ApparentTemperature == nil:
		return Current{}, missingField("forecast", "current.apparent_temperature")
	case cur.RelativeHumidity == nil:
		return Current{}, missingField("forecast", "current.relative_humidity_2m")
	case cur.WeatherCode == nil:
		return Current{}, missingField("forecast", "current.weather_code")
	case cur.WindSpeed == nil:
		return Current{}, missingField("forecast", "current.wind_speed_10m")
	}
	return Current{
		TemperatureC:         *cur.Temperature,
		ApparentTemperatureC: *cur.ApparentTemperature,
		RelativeHumidityPct:  *cur.RelativeHumidity,
		WindSpeedKmh:         *cur.WindSpeed,
		WeatherCode:          *cur.WeatherCode,
	}, nil
}

// get sends a GET request to base with the query and decodes a 2xx JSON response into out. Errors name the API but
// never the URL, whose query holds the searched city or the resolved coordinates.
func (c *Client) get(ctx context.Context, api, base string, query url.Values, out any) error {
	u, err := url.Parse(base)
	if err != nil {
		return fmt.Errorf("%s: %w: parse base URL: %v", api, ErrUpstream, err)
	}
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("%s: %w: build request: %v", api, ErrUpstream, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return classify(ctx, api, "send request", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return classify(ctx, api, "read response", err)
	}
	if len(data) > maxBodyBytes {
		return fmt.Errorf("%s: %w: response larger than %d bytes", api, ErrUpstream, maxBodyBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// Open-Meteo explains a rejected request as {"error": true, "reason": "..."}.
		var apiErr struct {
			Reason string `json:"reason"`
		}
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Reason != "" {
			return fmt.Errorf("%s: %w: status %d: %s", api, ErrUpstream, resp.StatusCode, apiErr.Reason)
		}
		return fmt.Errorf("%s: %w: status %d", api, ErrUpstream, resp.StatusCode)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s: %w: decode response: %v", api, ErrUpstream, err)
	}
	return nil
}

// classify wraps a transport error in ErrTimeout when the deadline passed and in ErrUpstream otherwise, dropping the
// request URL that net/http includes in its errors.
func classify(ctx context.Context, api, step string, err error) error {
	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		err = urlErr.Err
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s: %w: %s", api, ErrTimeout, step)
	}
	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		return fmt.Errorf("%s: %w: %s: %v", api, ErrTimeout, step, err)
	}
	return fmt.Errorf("%s: %w: %s: %v", api, ErrUpstream, step, err)
}

func missingField(api, field string) error {
	return fmt.Errorf("%s: %w: response has no %s", api, ErrUpstream, field)
}
