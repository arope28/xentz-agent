//go:build windows

package paths

func isElevated() bool {
	// Default to user mode unless explicitly requested.
	return false
}
