# Performance

This records the end-to-end latency measurement for AC-09: at least 95% of valid lookups must display their result
within 3 seconds, measured from submitting the search to displaying the result, with Open-Meteo available.

## Result

AC-09 is met. Every one of the 40 lookups displayed its result within 3 seconds, both when the service's connections
to Open-Meteo were already open and when every lookup had to open new ones.

| Scenario                      | Lookups | Results within 3 s | p50      | p95      | Max      |
|-------------------------------|---------|--------------------|----------|----------|----------|
| Open connections              | 40      | 40 (100%)          | 578 ms   | 1,050 ms | 2,080 ms |
| New connections every lookup  | 40      | 40 (100%)          | 2,082 ms | 2,242 ms | 2,382 ms |

## Method

The measurement is `TestLookupLatency` in `test/browser/perf_test.go`, run with `make perf` against the panel from
`docker compose up`.

- **Searches.** 40 distinct searches: 14 common names (such as São Paulo, Lisboa, and Tokyo), 14 accented names (such
  as Maceió, Zürich, and Kraków), and 12 names shared by several places (such as Santa Maria, Springfield, and
  Córdoba). All 40 resolved to a location.
- **Cold cache.** The test empties both cache tables before the first lookup. The searches are distinct, so every
  lookup misses the cache and calls both the Geocoding and the Forecast API.
- **Timing.** Inside the page, from the form's `submit` event to the first animation frame after htmx swaps the
  response into the result region. A search is typed and submitted with the keyboard, as a user would.
- **Rate.** Lookups start at least 3 seconds apart: at most 40 Open-Meteo calls a minute, well under the free tier's
  600.
- **Scenarios.** In the first, lookups follow each other, so the service reuses the connections it opened for the
  first one. In the second, `PERF_BEFORE_EACH='docker compose restart app'` restarts the service before every lookup,
  so each one resolves DNS and opens two new TLS connections, as the first lookup after an idle period does. Idle
  connections do not last long: the service closes its own after 90 seconds, and Open-Meteo closed some after about a
  minute in probes during development. Sparse real traffic therefore sees mostly the second scenario.

## Conditions

- 2026-09-24, around 01:40–01:55 UTC−4.
- Panel at commit `d6e74b4`, built and run by `docker compose up --build` (Go 1.27, PostgreSQL 18).
- Client: headless Chrome 153 driven by chromedp on the same machine as the panel, so the browser-to-panel network
  time is negligible. Ubuntu 24.04, Linux 7.0, 16 CPUs.
- Network: a residential connection with about 250 ms of round-trip time to both Open-Meteo hosts (TCP connect time
  measured with `curl`). Both hosts speak TLS 1.3 and HTTP/1.1 only.

## Observations

- A call to Open-Meteo over an open connection takes one round trip plus processing, about 250–350 ms. Over a new
  connection it takes about 1 second: DNS, the TCP handshake, the TLS handshake, and the request.
- With new connections, the service's own lookup took 2,059 ms at p50 and 2,366 ms at most, against a lookup deadline
  of 2.5 seconds. The margin is thin: on a slower day, some lookups after an idle period would hit the deadline and
  show the "unavailable" message. During development, one lookup did, when its geocoding call alone took 1.6 seconds.
- A cached lookup takes a few milliseconds on the server, so the cache removes the network cost entirely for repeated
  searches within its lifetimes.
- The results depend mostly on the round-trip time between the service and Open-Meteo's servers; deploying the
  service closer to them shortens every call. Opening the Forecast API connection while the Geocoding call is in
  flight would take about one second off the new-connection path; nothing does that today.

## Reproducing

```sh
docker compose up -d --build
make perf                                                         # open connections
PERF_BEFORE_EACH='docker compose restart app >/dev/null' make perf   # new connections every lookup
```

`BROWSER_TEST_URL` points the test at another panel, `PERF_DATABASE_URL` at its database, and `PERF_INTERVAL` changes
the spacing between lookups. The test fails when fewer than 95% of the lookups display a result within 3 seconds.
