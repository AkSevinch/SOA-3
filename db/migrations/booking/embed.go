package bookingmigrations

import "embed"

// FS содержит SQL миграции
//
//go:embed *.sql
var FS embed.FS
