-- name: GetGeocoding :one
SELECT found, location_id, name, admin1, country, latitude, longitude
FROM geocoding_cache
WHERE query = sqlc.arg(query) AND expires_at > sqlc.arg(now);

-- name: UpsertGeocoding :exec
INSERT INTO geocoding_cache (query, found, location_id, name, admin1, country, latitude, longitude, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (query) DO UPDATE SET
    found       = excluded.found,
    location_id = excluded.location_id,
    name        = excluded.name,
    admin1      = excluded.admin1,
    country     = excluded.country,
    latitude    = excluded.latitude,
    longitude   = excluded.longitude,
    expires_at  = excluded.expires_at;

-- name: GetWeather :one
SELECT temperature_c, apparent_temperature_c, relative_humidity_pct, wind_speed_kmh, weather_code
FROM weather_cache
WHERE location_id = sqlc.arg(location_id) AND expires_at > sqlc.arg(now);

-- name: UpsertWeather :exec
INSERT INTO weather_cache (
    location_id, temperature_c, apparent_temperature_c, relative_humidity_pct, wind_speed_kmh, weather_code, expires_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (location_id) DO UPDATE SET
    temperature_c          = excluded.temperature_c,
    apparent_temperature_c = excluded.apparent_temperature_c,
    relative_humidity_pct  = excluded.relative_humidity_pct,
    wind_speed_kmh         = excluded.wind_speed_kmh,
    weather_code           = excluded.weather_code,
    expires_at             = excluded.expires_at;

-- name: DeleteExpired :one
WITH geocoding AS (
    DELETE FROM geocoding_cache WHERE geocoding_cache.expires_at <= sqlc.arg(now) RETURNING 1
), weather AS (
    DELETE FROM weather_cache WHERE weather_cache.expires_at <= sqlc.arg(now) RETURNING 1
)
SELECT (SELECT count(*) FROM geocoding) AS geocoding_deleted, (SELECT count(*) FROM weather) AS weather_deleted;
