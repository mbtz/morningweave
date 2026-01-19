package storage

import _ "embed"

// Migration captures a single schema change.
type Migration struct {
	Name string
	SQL  string
}

//go:embed migrations/001_init.sql
var initSchema string

// Migrations returns the ordered migration list.
func Migrations() []Migration {
	return []Migration{
		{
			Name: "001_init",
			SQL:  initSchema,
		},
	}
}
