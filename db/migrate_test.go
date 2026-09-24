package db

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"

	"github.com/dev-labs-ai/weather-panel/internal/dbtest"
)

func TestMigrateFreshDatabase(t *testing.T) {
	t.Parallel()
	url := dbtest.Fresh(t)

	version, err := Migrate(url)
	if err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if version != 1 {
		t.Errorf("Migrate() version = %d, want 1", version)
	}
	assertTables(t, url, "geocoding_cache", "weather_cache")
}

func TestMigrateAlreadyApplied(t *testing.T) {
	t.Parallel()
	url := dbtest.Fresh(t)

	for i := range 2 {
		version, err := Migrate(url)
		if err != nil {
			t.Fatalf("Migrate() run %d error = %v", i+1, err)
		}
		if version != 1 {
			t.Errorf("Migrate() run %d version = %d, want 1", i+1, version)
		}
	}
}

func TestMigrateDownRemovesTheTables(t *testing.T) {
	t.Parallel()
	url := dbtest.Fresh(t)
	if _, err := Migrate(url); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	src, err := iofs.New(migrations, "migrations")
	if err != nil {
		t.Fatalf("iofs.New() error = %v", err)
	}
	migrateURL, err := pgx5URL(url)
	if err != nil {
		t.Fatalf("pgx5URL() error = %v", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, migrateURL)
	if err != nil {
		t.Fatalf("migrate.NewWithSourceInstance() error = %v", err)
	}
	defer m.Close()
	if err := m.Down(); err != nil {
		t.Fatalf("Down() error = %v", err)
	}
	assertTables(t, url)
}

func TestMigrateFailingMigration(t *testing.T) {
	t.Parallel()
	url := dbtest.Fresh(t)

	broken := fstest.MapFS{
		"migrations/000001_broken.up.sql":   {Data: []byte("CREATE TABLE broken (id integer PRIMARY KEY,);")},
		"migrations/000001_broken.down.sql": {Data: []byte("DROP TABLE broken;")},
	}
	if _, err := migrateFS(broken, "migrations", url); err == nil {
		t.Fatal("migrateFS() error = nil, want the syntax error")
	}
	// The failed migration leaves the schema dirty, so the next start fails too instead of running on it.
	if _, err := migrateFS(broken, "migrations", url); err == nil || !strings.Contains(err.Error(), "Dirty") {
		t.Errorf("second migrateFS() error = %v, want a dirty-database error", err)
	}
}

func TestMigrateUnreachableDatabase(t *testing.T) {
	t.Parallel()

	if _, err := Migrate(dbtest.UnreachableURL); err == nil {
		t.Fatal("Migrate() error = nil, want a connection error")
	}
}

func TestMigrateRejectsNonURL(t *testing.T) {
	t.Parallel()

	for _, dsn := range []string{"host=localhost user=weather", "mysql://weather@localhost/weather"} {
		_, err := Migrate(dsn)
		if err == nil || !strings.Contains(err.Error(), "postgres://") {
			t.Errorf("Migrate(%q) error = %v, want it to ask for a postgres:// URL", dsn, err)
		}
	}
}

// assertTables fails the test unless the public schema holds exactly the named tables, besides golang-migrate's own.
func assertTables(t *testing.T, url string, want ...string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name <> 'schema_migrations' ORDER BY table_name`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("tables = %v, want %v", got, want)
	}
}
