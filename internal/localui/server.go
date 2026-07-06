package localui

import (
	"context"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"xentz-agent/internal/config"
	"xentz-agent/internal/paths"
)

const tokenFileName = "local-ui.token"

type Server struct {
	addr  string
	token string
}

const restoreTimeout = 20 * time.Minute

func Start(addr string, cfgPath string) error {
	if addr == "" {
		addr = "127.0.0.1:9800"
	}
	token, err := ensureToken()
	if err != nil {
		return err
	}
	// While the local UI runs, keep config-cached.json updated for faster kill-switch visibility.
	config.StartAutoRefreshForConfigFile(context.Background(), cfgPath, "")
	s := &Server{addr: addr, token: token}
	return s.serve(cfgPath)
}

func (s *Server) serve(cfgPath string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot())
	mux.HandleFunc("/status", s.withAuth(s.handleStatus(cfgPath)))
	mux.HandleFunc("/config", s.withAuth(s.handleConfig(cfgPath)))
	mux.HandleFunc("/runs", s.withAuth(s.handleRuns()))
	mux.HandleFunc("/diagnostics", s.withAuth(s.handleDiagnostics()))
	mux.HandleFunc("/restore", s.withAuth(s.handleRestorePage()))
	mux.HandleFunc("/restore/snapshots", s.withAuth(s.handleRestoreSnapshots(cfgPath)))
	mux.HandleFunc("/restore/browse-local", s.withAuth(s.handleRestoreBrowseLocal()))
	mux.HandleFunc("/restore/browse-snapshot", s.withAuth(s.handleRestoreBrowseSnapshot(cfgPath)))
	mux.HandleFunc("/restore/plan", s.withAuth(s.handleRestorePlan(cfgPath)))
	mux.HandleFunc("/restore/run", s.withAuth(s.handleRestoreRun(cfgPath)))
	return http.ListenAndServe(s.addr, mux)
}

// handleRoot renders the polished dashboard using html/template (no fmt.Sprintf hacks).
func (s *Server) handleRoot() http.HandlerFunc {
	cfgDir, _ := paths.ConfigDir("")
	tokenPath := filepath.Join(cfgDir, tokenFileName)
	tok := url.QueryEscape(s.token)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		data := DashboardData{
			Token:     tok,
			RawToken:  s.token, // for JS (not escaped)
			TokenPath: tokenPath,
		}
		if err := dashboardTemplate.Execute(w, data); err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
		}
	}
}
