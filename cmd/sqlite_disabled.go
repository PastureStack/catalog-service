//go:build nosqlite
// +build nosqlite

package cmd

func sqliteAvailable() bool { return false }
