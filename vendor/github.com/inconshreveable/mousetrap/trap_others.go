//go:build !windows
// +build !windows

package mousetrap

// StartedByExplorer always returns false on non-Windows platforms.
func StartedByExplorer() bool {
	return false
}
