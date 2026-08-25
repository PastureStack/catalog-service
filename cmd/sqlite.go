//go:build !nosqlite
// +build !nosqlite

package cmd

import (
	gormsqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func sqliteAvailable() bool { return true }

func openSQLite(dsn string) (*gorm.DB, error) {
	return gorm.Open(gormsqlite.Open(dsn), newCatalogGormConfig())
}
