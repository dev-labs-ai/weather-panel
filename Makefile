BIN := bin/weather-panel

.PHONY: generate build run test

# Regenerates the committed sqlc and templ code.
generate:
	go tool templ generate
	go tool sqlc generate

build:
	go build -o $(BIN) ./cmd/weather-panel

run: build
	./$(BIN)

test:
	go test ./...
