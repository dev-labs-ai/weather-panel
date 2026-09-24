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

.PHONY: generate css build run test

# Regenerates the committed sqlc and templ code.
generate:
	go tool templ generate
	go tool sqlc generate

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
