package localui

import (
	"embed"
	"fmt"
	"html/template"
)

//go:embed templates/*.gohtml
var uiTemplatesFS embed.FS

var (
	dashboardTemplate *template.Template
	statusTmpl        *template.Template
	configTmpl        *template.Template
	restoreTmpl       *template.Template
)

func init() {
	dashBytes, err := uiTemplatesFS.ReadFile("templates/dashboard.gohtml")
	if err != nil {
		panic(fmt.Sprintf("failed to read dashboard template: %v", err))
	}
	dashboardTemplate = template.Must(template.New("dashboard").Parse(string(dashBytes)))

	statusBytes, err := uiTemplatesFS.ReadFile("templates/status.gohtml")
	if err != nil {
		panic(fmt.Sprintf("failed to read status template: %v", err))
	}
	statusTmpl = template.Must(template.New("status").Parse(string(statusBytes)))

	configBytes, err := uiTemplatesFS.ReadFile("templates/config.gohtml")
	if err != nil {
		panic(fmt.Sprintf("failed to read config template: %v", err))
	}
	configTmpl = template.Must(template.New("config").Parse(string(configBytes)))

	restoreBytes, err := uiTemplatesFS.ReadFile("templates/restore.gohtml")
	if err != nil {
		panic(fmt.Sprintf("failed to read restore template: %v", err))
	}
	restoreTmpl = template.Must(template.New("restore").Parse(string(restoreBytes)))
}

type DashboardData struct {
	Token     string
	RawToken  string
	TokenPath string
}

// StatusPageData is passed to the /status HTML template.
type StatusPageData struct {
	ServerURL         string
	ConfigRev         string
	ConfigFound       bool
	LastBackup        string
	LastBackupAt      string
	LastBackupStatus  string
	LastRetentions    string
	LastRetentionAt   string
	LastRetentionStat string
	Revoked           bool
	SpoolCount        int
	SpoolBytesHuman   string
}

func humanizeBytes(b int) string {
	unit := []string{"B", "KB", "MB", "GB", "TB"}
	if b == 0 {
		return "0 B"
	}
	i := 0
	f := float64(b)
	for f >= 1024 && i < len(unit)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", b)
	}
	return fmt.Sprintf("%.1f %s", f, unit[i])
}

// configPageData is passed to the /config HTML template.
type ConfigPageData struct {
	ServerURL    string
	TenantID     string
	DeviceID     string
	UserID       string
	ConfigRev    int
	EnableState  string // "true" or "false" for display, not sensitive values
	ScheduleCron string
	IncludePaths []string
	ExcludePaths []string
	ResticRepo   string
	Retention    string
	TokenPath    string
	PasswordFile string
	ConfigFound  bool
}

type RestorePageData struct {
	HomeDirJS template.JS
}
