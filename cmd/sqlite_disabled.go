//go:build nosqlite
// +build nosqlite

package cmd

import (
	"fmt"

	"gorm.io/gorm"
)

func sqliteAvailable() bool { return false }

func openSQLite(string) (*gorm.DB, error) {
	return nil, fmt.Errorf("SQLite support is not available in this binary")
}
