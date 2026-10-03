package cli

import "errors"

// exitError carries a desired process exit code for failures a command has
// already reported in its own user-facing format.
//
// Commands must return it instead of calling os.Exit, because os.Exit skips
// deferred cleanup - which is how buffered log writes get dropped on exactly
// the runs whose logs matter most (failed backups and retention runs).
type exitError struct {
	err  error
	code int
}

func (e *exitError) Error() string { return e.err.Error() }

func (e *exitError) Unwrap() error { return e.err }

// ExitCode returns the process exit code carried by err, defaulting to 1.
// The second return value reports whether the caller already reported the
// problem, in which case main must not print it again.
func ExitCode(err error) (int, bool) {
	var ue *usageError
	if errors.As(err, &ue) {
		return 2, true
	}
	var e *exitError
	if !errors.As(err, &e) {
		return 1, false
	}
	return e.code, true
}

// failWithCode wraps err so main exits with code without printing it again.
func failWithCode(err error, code int) error {
	return &exitError{err: err, code: code}
}
