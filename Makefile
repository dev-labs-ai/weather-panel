BIN := bin/weather-panel

.PHONY: build run test

build:
	go build -o $(BIN) ./cmd/weather-panel

run: build
	./$(BIN)

test:
	go test ./...
