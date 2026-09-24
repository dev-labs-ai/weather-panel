// Package db holds the database migrations, embedded in the binary, and applies them.
package db

import "embed"

// migrations holds the golang-migrate SQL files. sqlc reads the same files as the schema.
//
//go:embed migrations/*.sql
var migrations embed.FS
