// Command weather-panel runs the weather panel HTTP service.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dev-labs-ai/weather-panel/db"
	"github.com/dev-labs-ai/weather-panel/internal/cache"
	"github.com/dev-labs-ai/weather-panel/internal/config"
	"github.com/dev-labs-ai/weather-panel/internal/web"
	assets "github.com/dev-labs-ai/weather-panel/web"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	if err := run(ctx, os.Getenv, os.Stdout); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("service stopped", slog.Any("error", err))
		stop()
		os.Exit(1)
	}
}

// run serves HTTP until ctx is canceled, then stops accepting connections and waits for in-flight requests.
func run(ctx context.Context, getenv func(string) string, stdout io.Writer) error {
	cfg, err := config.Load(getenv)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	// A schema the service cannot rely on is fatal at startup; after that, the database is only a best-effort cache.
	version, err := db.Migrate(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	logger.Info("database migrated", slog.Uint64("version", uint64(version)))

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database pool: %w", err)
	}
	defer pool.Close()
	go cache.New(pool).RunCleanup(ctx, time.Hour, logger)

	// A request lasts at most the lookup deadline plus rendering, and shutdown waits that long for it to finish.
	requestTimeout := cfg.LookupTimeout + 5*time.Second
	srv := &http.Server{
		Handler:           web.NewRouter(logger, assets.Static),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      requestTimeout,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	ln, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(cfg.Port)))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	logger.Info("listening", slog.Int("port", cfg.Port))

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shut down: %w", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	logger.Info("stopped")
	return nil
}
