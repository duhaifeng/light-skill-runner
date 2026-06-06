// Package db embeds database migration scripts.
package db

import "embed"

// Migrations contains the SQL files in this directory.
//
//go:embed *.sql
var Migrations embed.FS
