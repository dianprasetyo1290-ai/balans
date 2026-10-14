// Package migrations menyimpan file migrasi goose yang di-embed ke binary migrate dan test.
package migrations

import "embed"

// FS berisi seluruh file migrasi SQL.
//
//go:embed *.sql
var FS embed.FS
