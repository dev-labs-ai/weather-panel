-- Answers from the Open-Meteo Geocoding API, keyed by the normalized, lower-cased search.
CREATE TABLE geocoding_cache (
    query       text PRIMARY KEY,
    found       boolean NOT NULL,  -- false caches a "not found" answer
    location_id bigint,
    name        text,
    admin1      text,
    country     text,
    latitude    double precision,
    longitude   double precision,
    expires_at  timestamptz NOT NULL
);

-- Current conditions from the Open-Meteo Forecast API, keyed by the Open-Meteo geocoding id.
CREATE TABLE weather_cache (
    location_id            bigint PRIMARY KEY,
    temperature_c          double precision NOT NULL,
    apparent_temperature_c double precision NOT NULL,
    relative_humidity_pct  double precision NOT NULL,
    wind_speed_kmh         double precision NOT NULL,
    weather_code           integer NOT NULL,
    expires_at             timestamptz NOT NULL
);

CREATE INDEX ON geocoding_cache (expires_at);
CREATE INDEX ON weather_cache (expires_at);
