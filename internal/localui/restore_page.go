package localui

import (
	"html/template"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func (s *Server) handleRestorePage() http.HandlerFunc {
	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		homeDir = "/tmp"
	}
	data := RestorePageData{
		HomeDirJS: template.JS(strconv.Quote(homeDir)),
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := restoreTmpl.Execute(w, data); err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
		}
	}
}
