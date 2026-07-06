package secretstore

import (
	"errors"
	"log"
	"os"
	"strings"
)

var ErrNotFound = errors.New("secret not found")

type Store interface {
	Get(key string) ([]byte, error)
	Put(key string, value []byte) error
	Delete(key string) error
}

var defaultStore Store = newStore()

// newStore selects the platform secret store, unless XENTZ_AGENT_SECRETSTORE=file
// forces the plain file store (0600 files under the config dir). The override
// exists for drills/CI that need HOME-scoped isolation; production installs
// should rely on the platform store.
func newStore() Store {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("XENTZ_AGENT_SECRETSTORE")), "file") {
		log.Printf("secretstore: XENTZ_AGENT_SECRETSTORE=file set; storing secrets as files protected only by permissions")
		return newFileStore()
	}
	return newPlatformStore()
}

func Get(key string) ([]byte, error) {
	return defaultStore.Get(key)
}

func Put(key string, value []byte) error {
	return defaultStore.Put(key, value)
}

func Delete(key string) error {
	return defaultStore.Delete(key)
}

const (
	KeyDeviceAPIKey   = "device_api_key"
	KeyResticPassword = "restic_password"
)
