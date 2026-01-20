package secretstore

import "errors"

var ErrNotFound = errors.New("secret not found")

type Store interface {
	Get(key string) ([]byte, error)
	Put(key string, value []byte) error
	Delete(key string) error
}

var defaultStore Store = newStore()

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
