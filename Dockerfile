# syntax=docker/dockerfile:1

# The builder runs on the target platform, so the Tailwind CLI it downloads for TARGETARCH can run in it.
FROM golang:1.27 AS build
ARG TARGETARCH
WORKDIR /src

COPY Makefile ./
RUN make tailwind TAILWIND_OS=linux TAILWIND_ARCH="$([ "$TARGETARCH" = arm64 ] && echo arm64 || echo x64)"

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN make css TAILWIND_OS=linux TAILWIND_ARCH="$([ "$TARGETARCH" = arm64 ] && echo arm64 || echo x64)" \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/weather-panel ./cmd/weather-panel

# distroless/static carries the CA certificates for the HTTPS calls to Open-Meteo and nothing else.
FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/weather-panel /weather-panel
EXPOSE 8080
ENTRYPOINT ["/weather-panel"]
