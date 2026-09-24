BIN := bin/weather-panel

# Tailwind CSS standalone CLI, pinned with the checksums from the release's sha256sums.txt.
TAILWIND_VERSION := v4.3.3
TAILWIND_SHA256_linux-x64 := dc61b3ac6b8c9ca874c0cc4c57b2409791a64c5540404ca5f5367360babc313a
TAILWIND_SHA256_linux-arm64 := 55fd0b241214eff3de1e8ee4f22796662f2d2e7a49bcfca7477cfd0bac398195
TAILWIND_SHA256_macos-x64 := 7922e0953f2110c05976e3bf58f14e643d90427575e766b7d433f5f80cbee7e1
TAILWIND_SHA256_macos-arm64 := cdf646702987a743464dff4d9c60fd4480d1c1e73dd819a9a67f1078815dce9d
TAILWIND_OS ?= $(if $(filter Darwin,$(shell uname -s)),macos,linux)
TAILWIND_ARCH ?= $(if $(filter arm64 aarch64,$(shell uname -m)),arm64,x64)
TAILWIND_PLATFORM := $(TAILWIND_OS)-$(TAILWIND_ARCH)
TAILWIND := bin/tailwindcss-$(TAILWIND_VERSION)-$(TAILWIND_PLATFORM)
SHA256SUM := $(if $(shell command -v sha256sum),sha256sum,shasum -a 256)

.PHONY: generate check-generated tailwind css build run test test-browser perf

# Regenerates the committed sqlc and templ code.
generate:
	go tool templ generate
	go tool sqlc generate

# Fails when the committed sqlc or templ code differs from what the pinned generators produce.
GENERATED := internal/store internal/view
check-generated: generate
	@if [ -n "$$(git status --porcelain --untracked-files=all -- $(GENERATED))" ]; then \
		echo "Generated code is stale; run 'make generate' and commit the result:"; \
		git status --short --untracked-files=all -- $(GENERATED); \
		git diff -- $(GENERATED); \
		exit 1; \
	fi

# Downloads the Tailwind CLI only; the Dockerfile runs it in its own layer so the download is cached.
tailwind: $(TAILWIND)

css: $(TAILWIND)
	$(TAILWIND) --input web/styles/app.css --output web/static/css/app.css --minify

$(TAILWIND):
	@test -n "$(TAILWIND_SHA256_$(TAILWIND_PLATFORM))" || { echo "no Tailwind checksum for $(TAILWIND_PLATFORM)"; exit 1; }
	mkdir -p bin
	curl -fsSL -o $@.tmp \
		https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-$(TAILWIND_PLATFORM)
	echo "$(TAILWIND_SHA256_$(TAILWIND_PLATFORM))  $@.tmp" | $(SHA256SUM) -c -
	chmod +x $@.tmp
	mv $@.tmp $@

build:
	go build -o $(BIN) ./cmd/weather-panel

run: build
	./$(BIN)

test:
	go test ./...

# Runs the browser tests against the panel at BROWSER_TEST_URL (default http://localhost:8080), such as the one
# `docker compose up -d --build` starts. Needs a local Chrome or Chromium.
test-browser:
	go test -tags browser -count=1 ./test/browser

# Measures AC-09 against the real Open-Meteo through the panel at BROWSER_TEST_URL, starting from an empty cache in
# PERF_DATABASE_URL. PERF_INTERVAL spaces the lookups; above a minute, every lookup opens new upstream connections.
perf:
	go test -tags browser,perf -count=1 -run TestLookupLatency -v -timeout 90m ./test/browser
