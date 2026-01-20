//go:build windows

package secretstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"xentz-agent/internal/paths"
)

type dpapiStore struct {
	dir string
}

func newStore() Store {
	dir, err := paths.ConfigDir("")
	if err != nil {
		return &dpapiStore{dir: ""}
	}
	return &dpapiStore{dir: filepath.Join(dir, "secrets")}
}

func (s *dpapiStore) Get(key string) ([]byte, error) {
	path, err := s.pathForKey(key)
	if err != nil {
		return nil, err
	}
	enc, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("read secret: %w", err)
	}
	dec, err := dpapiUnprotect(enc)
	if err != nil {
		return nil, fmt.Errorf("dpapi unprotect: %w", err)
	}
	return dec, nil
}

func (s *dpapiStore) Put(key string, value []byte) error {
	path, err := s.pathForKey(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create secret dir: %w", err)
	}
	enc, err := dpapiProtect(value)
	if err != nil {
		return fmt.Errorf("dpapi protect: %w", err)
	}
	if err := os.WriteFile(path, enc, 0o600); err != nil {
		return fmt.Errorf("write secret: %w", err)
	}
	return nil
}

func (s *dpapiStore) Delete(key string) error {
	path, err := s.pathForKey(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return fmt.Errorf("delete secret: %w", err)
	}
	return nil
}

func (s *dpapiStore) pathForKey(key string) (string, error) {
	if s.dir == "" {
		return "", fmt.Errorf("secret dir not available")
	}
	clean := sanitizeKey(key)
	if clean == "" {
		return "", fmt.Errorf("invalid secret key")
	}
	return filepath.Join(s.dir, clean+".bin"), nil
}

func sanitizeKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	key = strings.ReplaceAll(key, "/", "_")
	key = strings.ReplaceAll(key, "\\", "_")
	key = strings.ReplaceAll(key, "..", "_")
	return key
}

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func newBlob(d []byte) *dataBlob {
	if len(d) == 0 {
		return &dataBlob{}
	}
	return &dataBlob{
		cbData: uint32(len(d)),
		pbData: &d[0],
	}
}

func (b *dataBlob) bytes() []byte {
	if b.cbData == 0 || b.pbData == nil {
		return []byte{}
	}
	data := make([]byte, b.cbData)
	copy(data, (*[1 << 30]byte)(unsafe.Pointer(b.pbData))[:b.cbData:b.cbData])
	return data
}

var (
	crypt32                = syscall.NewLazyDLL("Crypt32.dll")
	kernel32               = syscall.NewLazyDLL("Kernel32.dll")
	procCryptProtectData   = crypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
	procLocalFree          = kernel32.NewProc("LocalFree")
)

func dpapiProtect(data []byte) ([]byte, error) {
	in := newBlob(data)
	var out dataBlob
	r, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(in)),
		0,
		0,
		0,
		0,
		0,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, err
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return out.bytes(), nil
}

func dpapiUnprotect(data []byte) ([]byte, error) {
	in := newBlob(data)
	var out dataBlob
	r, _, err := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(in)),
		0,
		0,
		0,
		0,
		0,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, err
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return out.bytes(), nil
}
