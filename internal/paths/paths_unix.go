//go:build !windows

package paths

import "os"

func isElevated() bool {
	return os.Geteuid() == 0
}
