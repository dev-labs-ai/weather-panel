# Implementation Tasks

This list turns the [PRD](prd.md) and the [technical specification](techspec.md) into ordered, reviewable units of
work. The techspec defines what each task builds; the PRD defines when it is done. Each task names the requirement and
acceptance criterion IDs it covers, and the [traceability matrix](#traceability) at the end checks that every one of
them is covered.

Tasks are listed in a workable order. Each should fit in a single pull request and leave the build and tests green.

**Scope guard.** Anything in the PRD's "Out of scope" section is not a task, even when the stack makes it cheap: no
automatic city suggestions, no geolocation, no choice between cities with the same name, no forecasts or history, no
unit or language switch.

## Phase 1: Foundation

- [x] **T01: Initialize the Go module and project skeleton**
  - Depends on: none
  - Create `go.mod` with module path `github.com/dev-labs-ai/weather-panel` and `go 1.27`.
  - Add templ and sqlc as `tool` directives at the versions pinned in the techspec.
  - Create the directory layout from the techspec and a `Makefile` with `build`, `run`, and `test`; later tasks add
    their own targets.
  - Ignore build outputs in `.gitignore` (binary, `web/static/css/app.css`).
  - Done when: `make build` and `make test` succeed, and `go tool templ version` and `go tool sqlc version` print the
    pinned versions.

- [x] **T02: Load configuration from the environment**
  - Depends on: T01
  - Implement `internal/config` with `PORT`, `DATABASE_URL`, `LOG_LEVEL`, `LOOKUP_TIMEOUT`,
    `OPEN_METEO_GEOCODING_URL`, and `OPEN_METEO_FORECAST_URL`, using the defaults from the techspec.
  - Done when: unit tests cover defaults, overrides, and startup failure when `DATABASE_URL` is missing or a value is
    malformed.

- [x] **T03: Build the HTTP server, middleware, and health check**
  - Depends on: T02
  - Covers: FR14, AC-04 (the CSP part)
  - `cmd/weather-panel/main.go` starts a chi router behind an `http.Server` with read-header, read, write, and idle
    timeouts, and shuts down gracefully on `SIGTERM`.
  - Add `GET /healthz`.
  - Add JSON logging through `log/slog` and a request logger that records method, route pattern, status, and duration.
  - Add a security-headers middleware that sets the CSP from the techspec, `X-Content-Type-Options: nosniff`, and
    `Referrer-Policy: no-referrer` on every response.
  - Done when: tests show every response carries the three headers, and a request with `?city=` produces a log line
    without the query string or the client IP.

## Phase 2: Domain and Open-Meteo integration

- [x] **T04: Normalize and validate the city input**
  - Depends on: T01
  - Covers: FR1, FR2, AC-05
  - In `internal/weather`: trim, collapse internal whitespace, normalize to NFC (`golang.org/x/text/unicode/norm`),
    require at least two letters or digits, cap at 100 runes, and derive the lower-cased cache key.
  - Return a `ValidationError` whose Brazilian Portuguese message explains how to fix the search.
  - Done when: table tests cover empty input, whitespace-only input, a single character, punctuation only, `Ré`,
    `São   Paulo`, decomposed accents that must equal their composed form, and 101 runes.

- [x] **T05: Map weather codes and format values for display**
  - Depends on: T01
  - Covers: FR4, FR5, FR6, FR7, FR8, AC-02, AC-03
  - Implement the WMO code table from the techspec, with "Condição não informada" for unknown codes.
  - Round temperatures and wind speed to whole numbers and humidity to a whole percentage (`23 °C`, `65%`, `12 km/h`),
    never producing `-0 °C`.
  - Format the location as `Name, Admin1, Country`, skipping missing parts.
  - Done when: unit tests cover every WMO code in the table, an unknown code, the rounding edges, and locations with and
    without `admin1` and `country`.

- [x] **T06: Implement the Open-Meteo client**
  - Depends on: T02
  - Covers: FR3, FR5, FR8, FR10, AC-01, AC-06, AC-07
  - In `internal/openmeteo`, add a Geocoding call (`count=1`, `language=pt`, `format=json`) and a Forecast call
    (`current=` with the five variables, explicit `temperature_unit=celsius` and `wind_speed_unit=kmh`), sharing one
    `http.Client`, with base URLs from configuration.
  - Read response bodies through a 1 MiB limit.
  - Classify outcomes:
    - a missing or empty `results` array means not found;
    - network errors, non-`2xx` statuses, malformed JSON, and missing required fields are upstream errors, and the
      `reason` of a `400` response is logged;
    - an exceeded context deadline is a timeout.
  - Done when: tests against an `httptest.Server` cover success, empty results, missing `results`, a `400` with a
    reason, a `500`, malformed JSON, each missing required field, an absent optional `admin1` or `country`, and a
    timeout.

- [x] **T07: Implement the lookup service**
  - Depends on: T04, T06
  - Covers: FR3, FR10, AC-01, AC-06, AC-07
  - The service normalizes the input, resolves the location, fetches the conditions, and returns a `Report` or one of
    `ValidationError`, `ErrNotFound`, `ErrUpstream`, and `ErrUpstreamTimeout`.
  - The whole lookup runs under the single `LOOKUP_TIMEOUT` deadline, with no retries.
  - The cache sits behind an interface; this task ships a no-op implementation.
  - Log each lookup with its outcome, cache hits, and upstream latencies, never the city or the resolved location.
  - Done when: tests with a fake client cover each outcome, and they show a slow upstream is cut off at the deadline.

## Phase 3: Cache

- [x] **T08: Add PostgreSQL to the local environment**
  - Depends on: T01
  - Create `compose.yaml` with the `db` service (`postgres:18-alpine`, a named volume, a `pg_isready` healthcheck).
    The `app` service comes in T16.
  - Done when: `docker compose up db` reports the database as healthy.

- [x] **T09: Run migrations at startup and open the connection pool**
  - Depends on: T03, T08
  - Add `db/migrations/000001_create_cache_tables.up.sql` and `.down.sql` with the schema from the techspec.
  - Embed them in `db/embed.go` and apply them at startup through golang-migrate (`iofs` source, `pgx5://` URL).
  - Open a `pgxpool` pool.
  - Done when: the service applies migrations against a fresh database, starts cleanly when they are already applied,
    and exits with an error when they fail.

- [x] **T10: Generate queries with sqlc and implement the PostgreSQL cache**
  - Depends on: T07, T09
  - Covers: AC-09 (latency); supports the PRD's provider-limit and data-retention constraints
  - Add `sqlc.yaml` (PostgreSQL engine, schema from `db/migrations`, queries from `db/queries`, `pgx/v5`, output in
    `internal/store`), the get and upsert queries for both tables (ignoring expired rows), and a delete-expired query.
    Add a `generate` target to the `Makefile`.
  - Implement `internal/cache` with lifetimes of 30 days for a found location, 1 day for not found, and 10 minutes for
    conditions. Upstream errors are never cached.
  - A database error is logged and treated as a cache miss; the lookup still completes.
  - An hourly job deletes expired rows.
  - Integration tests read `TEST_DATABASE_URL` and skip when it is unset.
  - Done when: integration tests cover hits, misses, expiry, upserts, not-found caching, and cleanup, and a service
    test shows a lookup succeeding while the database is down.

## Phase 4: Web interface

- [x] **T11: Serve static assets and set up the Tailwind pipeline**
  - Depends on: T03
  - Covers: FR14, AC-04 (same-origin assets)
  - Vendor `htmx.min.js` 4.0.0 into `web/static/js/`, embed `web/static` in `web/embed.go`, and serve it at
    `/static/*` with long-lived cache headers.
  - Create `web/styles/app.css` with `@import "tailwindcss";` and an `@source` pointing at `internal/view`.
  - Add a `css` target that downloads the pinned Tailwind standalone binary and builds `web/static/css/app.css`.
  - Done when: `make css build` produces a binary that serves both assets, and the page loads no resource from another
    origin.

- [x] **T12: Build the layout, page, and search form**
  - Depends on: T11
  - Covers: FR1, FR9, FR11, FR12, AC-08, AC-10, AC-11
  - Build a templ layout with `<html lang="pt-BR">`, a page title, and the CSS and htmx references.
  - Add a heading that explains the purpose of the panel.
  - Build the form from the techspec sketch:
    - a persistent label and a hint;
    - `hx-get`, `hx-target="#result"`, `hx-disable="find button"`, `hx-sync="this:drop"`, and `hx-indicator`;
    - a loading message.
  - Add the `#result` live region outside the form, so the input stays usable after any outcome.
  - `GET /` renders the full page.
  - Done when: the page renders, handler tests assert the form attributes and the live region, and the input is not
    disabled during a request.

- [x] **T13: Build the result and state components**
  - Depends on: T05, T12
  - Covers: FR2, FR4, FR5, FR6, FR7, FR8, FR10, FR13, AC-02, AC-03, AC-11, AC-13
  - Build a success component that shows the resolved location and highlights the temperature and condition, followed
    by feels-like temperature, humidity, and wind, each with a Brazilian Portuguese label.
  - End the success component with "Dados meteorológicos por [Open-Meteo.com](https://open-meteo.com/)".
  - Build three message components:
    - validation, with the element id `city-error`;
    - city not found;
    - temporarily unavailable.
  - Each message says what went wrong and what to do next, using no internal terms, no status codes, and no provider
    errors.
  - Nothing relies on an icon or a color alone.
  - Done when: component tests render every state with representative data and assert the labels, units, attribution
    link, and message texts.

- [x] **T14: Implement the `/weather` handler**
  - Depends on: T07, T13
  - Covers: FR1, FR2, FR3, FR10, FR11, FR12, FR13, AC-01, AC-02, AC-03, AC-05, AC-06, AC-07, AC-13
  - Read `city`, call the service, and map the outcome to the status codes in the techspec (`200`, `422`, `404`,
    `502`, `504`) and to the matching component.
  - Return the fragment when `HX-Request: true` is present and the full page otherwise; the full page keeps the typed
    city in the input.
  - Set `Vary: HX-Request`.
  - Wire the service, the cache, and the handler in `main.go`.
  - Done when: handler tests against a fake Open-Meteo server cover every outcome in both fragment and full-page modes,
    with status, `Vary`, and content checked. One test shows that an error response replaces a previous result
    entirely.

- [x] **T15: Style the panel for small screens, contrast, and focus**
  - Depends on: T13
  - Covers: AC-10, AC-12
  - Use a mobile-first Tailwind layout that works from 360 px wide, with WCAG AA contrast for text and controls,
    visible `focus-visible:` outlines, and emphasis on the temperature and condition.
  - Done when: a manual check at 360 px and 1280 px shows no horizontal scrolling, a contrast checker passes every text
    and control color, and focus is visible on every interactive element.

## Phase 5: Delivery

- [ ] **T16: Package the service with Docker**
  - Depends on: T10, T14, T15
  - Write a Dockerfile with a builder stage based on `golang:1.27` that fetches the Tailwind binary for
    `TARGETARCH`, builds the CSS, and compiles with `CGO_ENABLED=0`. The runtime stage is
    `gcr.io/distroless/static:nonroot`.
  - Add the `app` service to `compose.yaml`: it starts once `db` is healthy and receives `DATABASE_URL`.
  - Done when: `docker compose up --build` serves the panel on port 8080 and a real lookup works end to end.

- [ ] **T17: Add continuous integration**
  - Depends on: T10, T14
  - Add a GitHub Actions workflow on Go 1.27 that runs `make check-generated`, `go vet ./...`, and `go test ./...`,
    with a PostgreSQL service for the integration tests. Add the `check-generated` target to the `Makefile`.
  - Done when: the workflow passes on `main` and fails when generated code is stale.

## Phase 6: Verification

- [ ] **T18: Add automated browser tests**
  - Depends on: T16
  - Covers: FR9, FR14, AC-04, AC-08, AC-10, AC-12
  - Choose the browser test tool and record the choice in the techspec. The techspec leaves it open, and the project
    has no Node.js toolchain today.
  - Tests cover:
    - AC-04: every network request during a search goes to the application's origin;
    - AC-08: a double submission issues one request, and the button is disabled while the request is in flight;
    - AC-10: keyboard-only flow with a visible focus indicator;
    - AC-12: no horizontal scrolling at 360 px and 1280 px.
  - Done when: the tests run locally against `docker compose` and pass.

- [ ] **T19: Run a screen reader pass**
  - Depends on: T16
  - Covers: AC-11
  - Check loading, success, validation, not-found, and unavailable states with a screen reader.
  - If the loading indicator is not announced, move the loading message into the live region, as the techspec
    prescribes.
  - Done when: every state change is announced with its text, and the result is recorded in the pull request.

- [ ] **T20: Measure end-to-end performance**
  - Depends on: T18
  - Covers: AC-09
  - Measure a representative set of cities (common names, accented names, ambiguous names) against the real
    Open-Meteo, from submission to displayed result, with a cold cache. Keep the run well under the free tier's
    600 calls per minute.
  - Done when: at least 95% of lookups display the result within 3 seconds, and the measurement and its conditions are
    recorded.

- [ ] **T21: Update the README and CLAUDE.md with real commands**
  - Depends on: T16, T17
  - Replace "Getting started" in `README.md` and "Project status" in `CLAUDE.md` with the actual commands: build, run,
    test, a single test, generate, CSS, and Docker Compose.
  - Done when: someone following the README from a fresh clone gets the panel running.

## Traceability

Every PRD requirement and acceptance criterion is covered by at least one task.

| ID    | Tasks                       | ID    | Tasks                       |
|-------|-----------------------------|-------|-----------------------------|
| FR1   | T04, T12, T14               | AC-01 | T06, T07, T14               |
| FR2   | T04, T13, T14               | AC-02 | T05, T13, T14               |
| FR3   | T06, T07, T14               | AC-03 | T05, T13, T14               |
| FR4   | T05, T13                    | AC-04 | T03, T11, T18               |
| FR5   | T05, T06, T13               | AC-05 | T04, T14                    |
| FR6   | T05, T13                    | AC-06 | T06, T07, T14               |
| FR7   | T05, T13                    | AC-07 | T06, T07, T14               |
| FR8   | T05, T06, T13               | AC-08 | T12, T18                    |
| FR9   | T12, T18                    | AC-09 | T10, T20                    |
| FR10  | T06, T07, T13, T14          | AC-10 | T12, T15, T18               |
| FR11  | T12, T14                    | AC-11 | T12, T13, T19               |
| FR12  | T12, T14                    | AC-12 | T15, T18                    |
| FR13  | T13, T14                    | AC-13 | T13, T14                    |
| FR14  | T03, T11, T18               |       |                             |
