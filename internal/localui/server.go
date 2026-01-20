package localui

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"xentz-agent/internal/config"
	"xentz-agent/internal/diagnostics"
	"xentz-agent/internal/paths"
	"xentz-agent/internal/report"
	"xentz-agent/internal/state"
)

const tokenFileName = "local-ui.token"

type Server struct {
	addr  string
	token string
}

func Start(addr string, cfgPath string) error {
	if addr == "" {
		addr = "127.0.0.1:9800"
	}
	token, err := ensureToken()
	if err != nil {
		return err
	}
	s := &Server{addr: addr, token: token}
	return s.serve(cfgPath)
}

func (s *Server) serve(cfgPath string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", s.withAuth(s.handleStatus(cfgPath)))
	mux.HandleFunc("/config", s.withAuth(s.handleConfig(cfgPath)))
	mux.HandleFunc("/runs", s.withAuth(s.handleRuns()))
	mux.HandleFunc("/diagnostics", s.withAuth(s.handleDiagnostics()))
	return http.ListenAndServe(s.addr, mux)
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Local-Token")
		if token == "" || token != s.token {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("unauthorized"))
			return
		}
		next(w, r)
	}
}

func (s *Server) handleStatus(cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		st, _ := state.New()
		lastRun, _, _ := st.LoadLastRun()
		lastRetention, _, _ := st.LoadLastRetentionRun()
		agentState, _, _ := st.LoadAgentState()
		spoolCount, spoolBytes, _ := report.SpoolStats()

		resp := map[string]interface{}{
			"last_backup":    lastRun,
			"last_retention": lastRetention,
			"revoked":        agentState.Revoked,
			"spool_count":    spoolCount,
			"spool_bytes":    spoolBytes,
		}
		if cfg, err := config.Read(cfgPath); err == nil {
			resp["server_url"] = cfg.ServerURL
			resp["config_revision"] = cfg.ConfigRevision
		}

		writeJSON(w, resp)
	}
}

func (s *Server) handleConfig(cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		cfg, err := config.Read(cfgPath)
		if err != nil {
			writeErr(w, err)
			return
		}
		cfg.DeviceAPIKey = ""
		cfg.InstallToken = ""
		cfg.Restic.PasswordFile = ""
		writeJSON(w, cfg)
	}
}

func (s *Server) handleRuns() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		st, _ := state.New()
		lastRun, _, _ := st.LoadLastRun()
		lastRetention, _, _ := st.LoadLastRetentionRun()
		writeJSON(w, map[string]interface{}{
			"backup":    lastRun,
			"retention": lastRetention,
		})
	}
}

func (s *Server) handleDiagnostics() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		stateDir, err := paths.StateDir("")
		if err != nil {
			writeErr(w, err)
			return
		}
		outPath := filepath.Join(stateDir, "diagnostics.zip")
		if err := diagnostics.CreateBundle(outPath); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, map[string]string{"path": outPath})
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
