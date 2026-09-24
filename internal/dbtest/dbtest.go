// Package dbtest gives integration tests a PostgreSQL database of their own.
package dbtest

import (
	"context"
	"crypto/rand"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Fresh creates an empty database on the server at TEST_DATABASE_URL and returns its URL. The database is dropped
// when the test ends. Fresh skips the test when TEST_DATABASE_URL is unset, so `go test ./...` passes without
// PostgreSQL.
func Fresh(t testing.TB) string {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping the PostgreSQL integration test")
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal("TEST_DATABASE_URL is not a valid URL")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL: %v", err)
	}
	name := pgx.Identifier{"weather_test_" + rand.Text()}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name.Sanitize()); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+name.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("drop test database: %v", err)
		}
		_ = admin.Close(ctx)
	})

	u.Path = "/" + name[0]
	return u.String()
}

// UnreachableURL points at a port where no PostgreSQL listens, for tests of a database that is down.
const UnreachableURL = "postgres://weather:weather@127.0.0.1:1/weather?sslmode=disable&connect_timeout=1"
