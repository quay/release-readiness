package db

import (
	_ "embed"
	"fmt"
)

//go:embed schema.sql
var schemaSQL string

func (d *DB) migrate() error {
	if _, err := d.Exec(schemaSQL); err != nil {
		return fmt.Errorf("exec schema: %w", err)
	}
	return nil
}
