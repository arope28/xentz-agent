//go:build !darwin && !windows && !linux

package secretstore

// newPlatformStore is used on FreeBSD and other Unix-like systems that have no
// platform-specific secret store (keychain, DPAPI, libsecret).
func newPlatformStore() Store {
	return newFileStore()
}
