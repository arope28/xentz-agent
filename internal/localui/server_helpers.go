package localui

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"xentz-agent/internal/paths"
)

func (s *Server) templateData() map[string]interface{} {
	return map[string]interface{}{
		"Token":    s.token,
		"RawToken": url.QueryEscape(s.token),
	}
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Local-Token")
		if token == "" {
			token = r.URL.Query().Get("token") // allow ?token= for browser use
		}
		if token == "" || token != s.token {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("unauthorized"))
			return
		}
		next(w, r)
	}
}

func ensureToken() (string, error) {
	cfgDir, err := paths.ConfigDir("")
	if err != nil {
		return "", err
	}
	tokenPath := filepath.Join(cfgDir, tokenFileName)
	if data, err := os.ReadFile(tokenPath); err == nil {
		return string(data), nil
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return "", err
	}
	encoded := hex.EncodeToString(token)
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(tokenPath, []byte(encoded), 0o600); err != nil {
		return "", err
	}
	return encoded, nil
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte(fmt.Sprintf("error: %v", err)))
}
