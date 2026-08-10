package database

import (
	_ "embed"
)

//go:embed migrations.sql
var migrationsSQL string
