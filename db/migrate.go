package db

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the pgx5:// scheme
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Migrate applies every pending migration to the database at databaseURL, a postgres:// or postgresql:// URL, and
// returns the schema version it leaves the database at. It is safe to run when every migration is already applied,
// and concurrent runs wait for each other on a database lock.
func Migrate(databaseURL string) (uint, error) {
	return migrateFS(migrations, "migrations", databaseURL)
}

func migrateFS(fsys fs.FS, dir, databaseURL string) (version uint, err error) {
	migrateURL, err := pgx5URL(databaseURL)
	if err != nil {
		return 0, err
	}
	src, err := iofs.New(fsys, dir)
	if err != nil {
		return 0, fmt.Errorf("read migrations: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, migrateURL)
	if err != nil {
		return 0, fmt.Errorf("connect: %w", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if err == nil {
			err = errors.Join(srcErr, dbErr)
		}
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return 0, fmt.Errorf("apply migrations: %w", err)
	}
	version, _, err = m.Version()
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}

// pgx5URL turns a PostgreSQL URL into the form golang-migrate's pgx v5 driver registers.
func pgx5URL(databaseURL string) (string, error) {
	u, err := url.Parse(databaseURL)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return "", errors.New("DATABASE_URL must be a postgres:// or postgresql:// URL")
	}
	u.Scheme = "pgx5"
	return u.String(), nil
}
