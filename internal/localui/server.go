package localui

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"xentz-agent/internal/backup"
	"xentz-agent/internal/config"
	"xentz-agent/internal/diagnostics"
	"xentz-agent/internal/paths"
	"xentz-agent/internal/report"
	"xentz-agent/internal/state"
)

const tokenFileName = "local-ui.token"

//go:embed templates/dashboard.gohtml
var dashboardHTML embed.FS

var (
dashboardTemplate *template.Template
)

func init() {
	dashBytes, err := dashboardHTML.ReadFile("templates/dashboard.gohtml")
	if err != nil {
		panic(fmt.Sprintf("failed to read dashboard template: %v", err))
	}
	dashboardTemplate = template.Must(template.New("dashboard").Parse(string(dashBytes)))
}

type DashboardData struct {
	Token     string
	RawToken  string
	TokenPath string
}

type Server struct {
	addr  string
	token string
}

type restoreRequest struct {
	Type             string `json:"type"` // file|folder|snapshot
	SnapshotID       string `json:"snapshot_id"`
	Path             string `json:"path,omitempty"`
	Target           string `json:"target"`
	ConfirmOverwrite bool   `json:"confirm_overwrite,omitempty"`
}

type browseEntry struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Type  string `json:"type"` // file|dir
	IsDir bool   `json:"is_dir"`
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

func (s *Server) handleRestorePage() http.HandlerFunc {
	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		homeDir = "/tmp"
	}
	html := `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>xentz-agent Restore Wizard</title>
  <style>
    *, *::before, *::after { box-sizing: border-box; }
    :root {
      --bg: #f8fafc;
      --card: #ffffff;
      --text: #0f172a;
      --muted: #475569;
      --line: #e2e8f0;
      --ok: #166534;
      --warn: #92400e;
      --err: #991b1b;
      --blue: #0f4ac6;
      --blue-soft: #e8f0ff;
    }
    body { background: var(--bg); color: var(--text); font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; margin: 24px auto; max-width: 980px; line-height: 1.4; }
    h1 { margin: 0 0 6px; }
    .muted { color: var(--muted); margin: 0; }
    .layout { display: grid; grid-template-columns: 2fr 1fr; gap: 16px; margin-top: 16px; }
    .card { background: var(--card); border: 1px solid var(--line); border-radius: 12px; padding: 16px; }
    .step { border-left: 3px solid var(--line); padding-left: 12px; margin-bottom: 14px; }
    .step.active { border-left-color: var(--blue); background: var(--blue-soft); border-radius: 8px; padding: 10px 12px; }
    .step h3 { margin: 0 0 8px; font-size: 16px; }
    .row { display: grid; grid-template-columns: 170px minmax(0, 1fr); gap: 10px; align-items: center; margin: 10px 0; }
    input, select, button { font: inherit; padding: 8px 10px; border-radius: 8px; border: 1px solid var(--line); background: #fff; color: inherit; }
    input:focus, select:focus, button:focus { outline: 2px solid #b8d2ff; outline-offset: 1px; }
    button { cursor: pointer; }
    button.primary { background: var(--blue); color: #fff; border-color: var(--blue); }
    button:disabled { opacity: 0.6; cursor: not-allowed; }
    .actions { display: flex; gap: 10px; flex-wrap: wrap; margin-top: 12px; }
    .browse-box { border: 1px solid var(--line); border-radius: 8px; background: #fff; margin-top: 8px; }
    .browse-header { padding: 8px; border-bottom: 1px solid var(--line); font-size: 12px; color: var(--muted); overflow-wrap:anywhere; word-break:break-word; }
    .browse-list { max-height: 220px; overflow: auto; }
    .browse-item { display:flex; align-items:center; justify-content:space-between; gap:8px; padding:8px; border-bottom:1px solid #eef2f7; }
    .browse-item:last-child { border-bottom:0; }
    .browse-item .name { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
    .browse-item .item-actions { display:flex; gap:6px; flex-shrink:0; }
    .browse-item button { padding:4px 8px; font-size:12px; }
    .chip { display: inline-block; border-radius: 999px; padding: 2px 10px; font-size: 12px; border: 1px solid var(--line); color: var(--muted); }
    .chip.ok { color: var(--ok); border-color: #bbf7d0; background: #f0fdf4; }
    .chip.warn { color: var(--warn); border-color: #fde68a; background: #fffbeb; }
    .chip.err { color: var(--err); border-color: #fecaca; background: #fef2f2; }
    pre { white-space: pre-wrap; word-break: break-word; margin: 0; font-size: 12px; background: #f8fafc; border: 1px solid var(--line); border-radius: 8px; padding: 10px; min-height: 64px; }
    .hint { font-size: 12px; color: var(--muted); margin-top: 6px; }
    .hidden { display: none; }
    @media (max-width: 900px) { .layout { grid-template-columns: 1fr; } .row { grid-template-columns: 1fr; } }
  </style>
</head>
<body>
  <h1>Restore Wizard</h1>
  <p class="muted">Use this guided flow to safely restore one file, one folder, or a full snapshot.</p>

  <div class="layout">
    <div class="card">
      <div id="step-1" class="step active">
        <h3>Step 1: Choose what to restore</h3>
        <div class="row">
          <label for="type">Restore type</label>
          <select id="type">
            <option value="file">One file</option>
            <option value="folder">One folder</option>
            <option value="snapshot">Full snapshot</option>
          </select>
        </div>
        <div class="row">
          <label for="snapshot">Snapshot</label>
          <select id="snapshot">
            <option value="latest">latest</option>
          </select>
        </div>
        <div class="hint">Tip: choose <code>latest</code> unless you need an older version.</div>
      </div>

      <div id="step-2" class="step">
        <h3>Step 2: Choose source and destination</h3>
        <div class="row" id="path-row">
          <label for="path">Source path</label>
          <div>
            <input id="path" placeholder="/Users/me/Documents/file-or-folder" style="width:100%">
            <div class="actions" style="margin-top:6px">
              <button id="btn-browse-source" type="button">Browse snapshot source</button>
              <button id="btn-close-source" type="button" class="hidden">Close browser</button>
            </div>
            <div id="source-browser" class="browse-box hidden">
              <div id="source-browse-path" class="browse-header">Snapshot browser not open.</div>
              <div id="source-browse-list" class="browse-list"></div>
            </div>
          </div>
        </div>
        <div class="row">
          <label for="target">Destination path</label>
          <div>
            <input id="target" placeholder="/Users/me/Desktop/xentz-restore-YYYYMMDD-HHMMSS" style="width:100%">
            <div class="actions" style="margin-top:6px">
              <button id="btn-browse-target" type="button">Browse local destination</button>
              <button id="btn-close-target" type="button" class="hidden">Close browser</button>
            </div>
            <div id="target-browser" class="browse-box hidden">
              <div id="target-browse-path" class="browse-header">Local browser not open.</div>
              <div id="target-browse-list" class="browse-list"></div>
            </div>
          </div>
        </div>
        <div class="row">
          <label for="overwrite">Allow overwrite/merge</label>
          <input id="overwrite" type="checkbox">
        </div>
        <div class="hint">Safer default: restore into a new Desktop folder and review files before moving them back.</div>
      </div>

      <div id="step-3" class="step">
        <h3>Step 3: Preview and run</h3>
        <div class="actions">
          <button id="btn-load">Reload snapshots</button>
          <button id="btn-plan">Preview restore plan</button>
          <button id="btn-run" class="primary" disabled>Run restore</button>
        </div>
        <div class="hint">Run is enabled after a successful plan preview.</div>
      </div>
    </div>

    <div class="card">
      <h3 style="margin-top:0">Status</h3>
      <div id="status-chip" class="chip">Ready</div>
      <p class="hint" id="status-text">Load snapshots, then preview your plan.</p>
      <div style="margin-top:12px">
        <h4 style="margin:0 0 6px">Plan preview</h4>
        <pre id="plan">No plan yet.</pre>
      </div>
      <div style="margin-top:12px">
        <h4 style="margin:0 0 6px">Restore result</h4>
        <pre id="result">No restore run yet.</pre>
      </div>
    </div>
  </div>

  <script>
    const homeDir = ` + fmt.Sprintf("%q", homeDir) + `;
    const token = new URLSearchParams(window.location.search).get("token") || "";
    const snapshotEl = document.getElementById("snapshot");
    const typeEl = document.getElementById("type");
    const pathRowEl = document.getElementById("path-row");
    const pathEl = document.getElementById("path");
    const targetEl = document.getElementById("target");
    const overwriteEl = document.getElementById("overwrite");
    const planEl = document.getElementById("plan");
    const resultEl = document.getElementById("result");
    const sourceBrowserEl = document.getElementById("source-browser");
    const sourceBrowsePathEl = document.getElementById("source-browse-path");
    const sourceBrowseListEl = document.getElementById("source-browse-list");
    const targetBrowserEl = document.getElementById("target-browser");
    const targetBrowsePathEl = document.getElementById("target-browse-path");
    const targetBrowseListEl = document.getElementById("target-browse-list");
    const closeSourceBtn = document.getElementById("btn-close-source");
    const closeTargetBtn = document.getElementById("btn-close-target");
    const runBtn = document.getElementById("btn-run");
    const statusChip = document.getElementById("status-chip");
    const statusText = document.getElementById("status-text");
    let lastPlanOk = false;

    function endpoint(path) {
      if (!token) return path;
      const q = (path.includes("?") ? "&" : "?") + "token=" + encodeURIComponent(token);
      return path + q;
    }

    function payload() {
      return {
        type: typeEl.value,
        snapshot_id: snapshotEl.value || "latest",
        path: pathEl.value.trim(),
        target: targetEl.value.trim(),
        confirm_overwrite: overwriteEl.checked
      };
    }

    function setStatus(kind, text) {
      statusChip.className = "chip " + kind;
      statusChip.textContent = kind === "ok" ? "Ready" : kind === "warn" ? "Needs Attention" : kind === "err" ? "Error" : "Working";
      statusText.textContent = text;
    }

    function defaultTarget() {
      const stamp = new Date().toISOString().replace(/[-:]/g, "").replace("T", "-").slice(0, 15);
      const base = (homeDir || "/tmp") + "/Desktop";
      if (typeEl.value === "file") return base + "/xentz-restore-file-" + stamp;
      if (typeEl.value === "folder") return base + "/xentz-restore-folder-" + stamp;
      return base + "/xentz-restore-snapshot-" + stamp;
    }

    function setStep(n) {
      ["step-1", "step-2", "step-3"].forEach((id, idx) => {
        const el = document.getElementById(id);
        if (!el) return;
        if (idx + 1 === n) el.classList.add("active"); else el.classList.remove("active");
      });
    }

    function syncPathVisibility() {
      const needsPath = typeEl.value === "file" || typeEl.value === "folder";
      pathRowEl.style.display = needsPath ? "grid" : "none";
      if (!targetEl.value.trim()) {
        targetEl.value = defaultTarget();
      }
      lastPlanOk = false;
      runBtn.disabled = true;
      setStep(2);
    }

    async function fetchJSON(path, init) {
      const res = await fetch(endpoint(path), init);
      const raw = await res.text();
      let data = null;
      try { data = raw ? JSON.parse(raw) : null; } catch (_) {}
      if (res.status === 401) {
        throw new Error("unauthorized (missing/invalid local token in URL)");
      }
      if (!res.ok) {
        const msg = data && data.message ? data.message : (raw || ("HTTP " + res.status));
        throw new Error(msg);
      }
      return data;
    }

    async function loadSnapshots() {
      resultEl.textContent = "Loading snapshots...";
      setStatus("", "Loading restore snapshots...");
      setStep(1);
      try {
        const data = await fetchJSON("/restore/snapshots");
        snapshotEl.innerHTML = "";
        const latest = document.createElement("option");
        latest.value = "latest";
        latest.textContent = "latest";
        snapshotEl.appendChild(latest);
        (Array.isArray(data) ? data : []).slice(0, 50).forEach((s) => {
          const id = s.short_id || s.id;
          const label = (id || "unknown") + (s.time ? ("  (" + s.time + ")") : "");
          const opt = document.createElement("option");
          opt.value = s.id || "latest";
          opt.textContent = label;
          snapshotEl.appendChild(opt);
        });
        resultEl.textContent = "Snapshots loaded.";
        setStatus("ok", "Snapshots loaded. Continue to source/destination.");
      } catch (e) {
        resultEl.textContent = "Snapshot load failed: " + e.message;
        setStatus("err", "Snapshot load failed.");
      }
    }

    function renderBrowseList(listEl, entries, onOpen, onUse, canUse) {
      listEl.innerHTML = "";
      if (!entries || entries.length === 0) {
        const empty = document.createElement("div");
        empty.style.padding = "8px";
        empty.textContent = "(empty)";
        listEl.appendChild(empty);
        return;
      }
      entries.forEach((e) => {
        const row = document.createElement("div");
        row.className = "browse-item";

        const name = document.createElement("div");
        name.className = "name";
        name.title = e.path;
        name.textContent = (e.is_dir ? "📁 " : "📄 ") + e.name;
        row.appendChild(name);

        const actions = document.createElement("div");
        actions.className = "item-actions";
        if (e.is_dir) {
          const openBtn = document.createElement("button");
          openBtn.type = "button";
          openBtn.textContent = "Open";
          openBtn.onclick = () => onOpen(e);
          actions.appendChild(openBtn);
        }
        const useBtn = document.createElement("button");
        useBtn.type = "button";
        useBtn.textContent = e.is_dir ? "Use folder" : "Use path";
        const allowed = canUse ? !!canUse(e) : true;
        useBtn.disabled = !allowed;
        if (!allowed) {
          useBtn.title = "Not valid for current restore type";
        }
        useBtn.onclick = () => onUse(e);
        actions.appendChild(useBtn);

        row.appendChild(actions);
        listEl.appendChild(row);
      });
    }

    async function browseLocal(path) {
      try {
        const data = await fetchJSON("/restore/browse-local?path=" + encodeURIComponent(path || ""));
        targetBrowserEl.classList.remove("hidden");
        sourceBrowserEl.classList.add("hidden");
        closeTargetBtn.classList.remove("hidden");
        closeSourceBtn.classList.add("hidden");
        targetBrowsePathEl.textContent = "Local: " + data.path;
        const items = [];
        if (data.parent) {
          items.push({ name: "..", path: data.parent, is_dir: true, type: "dir" });
        }
        (data.entries || []).forEach(e => items.push(e));
        renderBrowseList(targetBrowseListEl, items, (entry) => {
          if (entry.is_dir) browseLocal(entry.path);
        }, (entry) => {
          targetEl.value = entry.path;
          lastPlanOk = false;
          runBtn.disabled = true;
          setStatus("warn", "Destination updated. Preview plan again.");
          if (entry.is_dir) browseLocal(entry.path);
        }, () => true);
      } catch (e) {
        targetBrowsePathEl.textContent = "Browse failed: " + e.message;
        targetBrowseListEl.innerHTML = "";
      }
    }

    async function browseSnapshot(path) {
      try {
        const snap = snapshotEl.value || "latest";
        const data = await fetchJSON("/restore/browse-snapshot?snapshot_id=" + encodeURIComponent(snap) + "&path=" + encodeURIComponent(path || "/"));
        sourceBrowserEl.classList.remove("hidden");
        targetBrowserEl.classList.add("hidden");
        closeSourceBtn.classList.remove("hidden");
        closeTargetBtn.classList.add("hidden");
        sourceBrowsePathEl.textContent = "Snapshot (" + snap + "): " + data.path;
        const items = [];
        if (data.parent && data.path !== "/") {
          items.push({ name: "..", path: data.parent, is_dir: true, type: "dir" });
        }
        (data.entries || []).forEach(e => items.push(e));
        renderBrowseList(sourceBrowseListEl, items, (entry) => {
          if (entry.is_dir) {
            pathEl.value = entry.path;
            browseSnapshot(entry.path);
          }
        }, (entry) => {
          const t = typeEl.value;
          if (t === "file" && entry.is_dir) {
            setStatus("warn", "For file restore, choose a file path (or open folder then pick a file).");
            return;
          }
          if (t === "folder" && !entry.is_dir) {
            setStatus("warn", "For folder restore, choose a folder path.");
            return;
          }
          pathEl.value = entry.path;
          lastPlanOk = false;
          runBtn.disabled = true;
          setStatus("warn", "Source path updated. Preview plan again.");
          if (entry.is_dir) browseSnapshot(entry.path);
        }, (entry) => {
          const t = typeEl.value;
          if (t === "file") return !entry.is_dir;
          if (t === "folder") return entry.is_dir;
          return true;
        });
      } catch (e) {
        sourceBrowsePathEl.textContent = "Snapshot browse failed: " + e.message;
        sourceBrowseListEl.innerHTML = "";
      }
    }

    async function planRestore() {
      planEl.textContent = "Planning...";
      setStatus("", "Validating restore request...");
      setStep(3);
      if (!targetEl.value.trim()) {
        targetEl.value = defaultTarget();
      }
      try {
        const data = await fetchJSON("/restore/plan", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload())
        });
        planEl.textContent = JSON.stringify(data, null, 2);
        lastPlanOk = !!data.ok;
        runBtn.disabled = !lastPlanOk;
        if (data.ok && data.confirm_required) {
          setStatus("warn", "Plan ok, but target may be overwritten. Enable overwrite/merge to continue.");
        } else if (data.ok) {
          setStatus("ok", "Plan looks good. You can run restore.");
        } else {
          setStatus("err", "Plan failed. Fix errors and preview again.");
        }
      } catch (e) {
        lastPlanOk = false;
        runBtn.disabled = true;
        planEl.textContent = "Plan failed: " + e.message;
        setStatus("err", "Plan request failed.");
      }
    }

    async function runRestore() {
      if (!lastPlanOk) {
        setStatus("warn", "Preview plan first before running restore.");
        return;
      }
      resultEl.textContent = "Running restore...";
      runBtn.disabled = true;
      setStatus("", "Restore is running...");
      try {
        const data = await fetchJSON("/restore/run", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload())
        });
        resultEl.textContent = JSON.stringify(data, null, 2);
        setStatus("ok", "Restore finished successfully.");
      } catch (e) {
        resultEl.textContent = "Restore failed: " + e.message;
        setStatus("err", "Restore failed.");
      } finally {
        runBtn.disabled = false;
      }
    }

    typeEl.addEventListener("change", syncPathVisibility);
    snapshotEl.addEventListener("change", () => { lastPlanOk = false; runBtn.disabled = true; setStatus("warn", "Snapshot changed. Preview plan again."); });
    pathEl.addEventListener("input", () => { lastPlanOk = false; runBtn.disabled = true; });
    targetEl.addEventListener("input", () => { lastPlanOk = false; runBtn.disabled = true; });
    overwriteEl.addEventListener("change", () => { lastPlanOk = false; runBtn.disabled = true; });
    document.getElementById("btn-load").addEventListener("click", loadSnapshots);
    document.getElementById("btn-plan").addEventListener("click", planRestore);
    document.getElementById("btn-run").addEventListener("click", runRestore);
    document.getElementById("btn-browse-target").addEventListener("click", () => {
      browseLocal(targetEl.value.trim() || homeDir + "/Desktop");
    });
    document.getElementById("btn-browse-source").addEventListener("click", () => {
      browseSnapshot(pathEl.value.trim() || "/");
    });
    closeSourceBtn.addEventListener("click", () => {
      sourceBrowserEl.classList.add("hidden");
      closeSourceBtn.classList.add("hidden");
    });
    closeTargetBtn.addEventListener("click", () => {
      targetBrowserEl.classList.add("hidden");
      closeTargetBtn.classList.add("hidden");
    });
    targetEl.value = defaultTarget();
    syncPathVisibility();
    loadSnapshots();
  </script>
</body>
</html>`
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
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

func (s *Server) handleRestoreSnapshots(cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		env, err := resticEnvForUI(cfgPath)
		if err != nil {
			writeErr(w, err)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), restoreTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "restic", "snapshots", "--json")
		cmd.Env = env
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			logRestoreDetail("RESTORE_SNAPSHOTS_FAILED", fmt.Errorf("%w (%s)", err, tailText(stderr.String(), 1024)))
			writeRestoreError(w, http.StatusInternalServerError, "RESTORE_SNAPSHOTS_FAILED", "failed to list snapshots")
			return
		}
		var parsed interface{}
		if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
			logRestoreDetail("RESTORE_SNAPSHOTS_PARSE_FAILED", err)
			writeRestoreError(w, http.StatusInternalServerError, "RESTORE_SNAPSHOTS_PARSE_FAILED", "failed to parse snapshots")
			return
		}
		writeJSON(w, parsed)
	}
}

func (s *Server) handleRestoreBrowseLocal() http.HandlerFunc {
	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		homeDir = "/tmp"
	}
	resolvedHome := homeDir
	if h, err := filepath.EvalSymlinks(homeDir); err == nil && strings.TrimSpace(h) != "" {
		resolvedHome = h
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		rawPath := strings.TrimSpace(r.URL.Query().Get("path"))
		if rawPath == "" {
			rawPath = homeDir
		}
		p := filepath.Clean(rawPath)
		if !filepath.IsAbs(p) {
			p = filepath.Join(homeDir, p)
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			writeRestoreError(w, http.StatusBadRequest, "RESTORE_BROWSE_LOCAL_BAD_PATH", "invalid local path")
			return
		}
		resolvedPath, err := filepath.EvalSymlinks(abs)
		if err != nil {
			writeRestoreError(w, http.StatusBadRequest, "RESTORE_BROWSE_LOCAL_BAD_PATH", "cannot resolve local path")
			return
		}
		if !isSubPath(resolvedPath, resolvedHome) {
			writeRestoreError(w, http.StatusForbidden, "RESTORE_BROWSE_LOCAL_FORBIDDEN", "path outside home directory is not allowed")
			return
		}
		items, err := os.ReadDir(resolvedPath)
		if err != nil {
			writeRestoreError(w, http.StatusBadRequest, "RESTORE_BROWSE_LOCAL_READ_FAILED", "cannot read local path")
			return
		}
		entries := make([]browseEntry, 0, len(items))
		for _, item := range items {
			name := item.Name()
			child := filepath.Join(resolvedPath, name)
			entries = append(entries, browseEntry{
				Path:  child,
				Name:  name,
				Type:  map[bool]string{true: "dir", false: "file"}[item.IsDir()],
				IsDir: item.IsDir(),
			})
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].IsDir != entries[j].IsDir {
				return entries[i].IsDir
			}
			return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		})
		parent := filepath.Dir(resolvedPath)
		if !isSubPath(parent, resolvedHome) {
			parent = ""
		}
		writeJSON(w, map[string]interface{}{
			"path":    resolvedPath,
			"parent":  parent,
			"entries": entries,
		})
	}
}

func (s *Server) handleRestoreBrowseSnapshot(cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		snapshotID := strings.TrimSpace(r.URL.Query().Get("snapshot_id"))
		if snapshotID == "" {
			snapshotID = "latest"
		}
		basePath := strings.TrimSpace(r.URL.Query().Get("path"))
		if basePath == "" {
			basePath = "/"
		}
		basePath = filepath.Clean(basePath)
		if !filepath.IsAbs(basePath) {
			basePath = "/" + strings.TrimLeft(basePath, "/")
		}

		env, err := resticEnvForUI(cfgPath)
		if err != nil {
			writeRestoreError(w, http.StatusInternalServerError, "RESTORE_BROWSE_SNAPSHOT_ENV_FAILED", "restore environment unavailable")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), restoreTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "restic", "ls", snapshotID, basePath, "--json")
		cmd.Env = env
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			logRestoreDetail("RESTORE_BROWSE_SNAPSHOT_FAILED", fmt.Errorf("%w (%s)", err, tailText(stderr.String(), 1024)))
			writeRestoreError(w, http.StatusBadRequest, "RESTORE_BROWSE_SNAPSHOT_FAILED", "cannot browse snapshot path")
			return
		}

		children := make(map[string]browseEntry)
		sc := bufio.NewScanner(bytes.NewReader(stdout.Bytes()))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			var obj map[string]interface{}
			if err := json.Unmarshal([]byte(line), &obj); err != nil {
				continue
			}
			if st, _ := obj["struct_type"].(string); st != "node" {
				continue
			}
			p, _ := obj["path"].(string)
			if p == "" || p == basePath {
				continue
			}
			p = filepath.Clean(p)
			if !isSubPath(p, basePath) {
				continue
			}
			rel := strings.TrimPrefix(p, basePath)
			rel = strings.TrimPrefix(rel, "/")
			if rel == "" {
				continue
			}
			parts := strings.Split(rel, "/")
			childName := parts[0]
			childPath := filepath.Join(basePath, childName)
			isDir := len(parts) > 1
			if !isDir {
				t, _ := obj["type"].(string)
				isDir = t == "dir"
			}
			existing, ok := children[childPath]
			if ok {
				existing.IsDir = existing.IsDir || isDir
				existing.Type = map[bool]string{true: "dir", false: "file"}[existing.IsDir]
				children[childPath] = existing
				continue
			}
			children[childPath] = browseEntry{
				Path:  childPath,
				Name:  childName,
				Type:  map[bool]string{true: "dir", false: "file"}[isDir],
				IsDir: isDir,
			}
		}
		entries := make([]browseEntry, 0, len(children))
		for _, e := range children {
			entries = append(entries, e)
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].IsDir != entries[j].IsDir {
				return entries[i].IsDir
			}
			return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
		})
		parent := filepath.Dir(basePath)
		if basePath == "/" {
			parent = "/"
		}
		writeJSON(w, map[string]interface{}{
			"path":       basePath,
			"parent":     parent,
			"entries":    entries,
			"snapshot_id": snapshotID,
		})
	}
}

func (s *Server) handleRestorePlan(cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req restoreRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeRestoreError(w, http.StatusBadRequest, "RESTORE_BAD_REQUEST", "invalid JSON request")
			return
		}
		if req.SnapshotID == "" {
			req.SnapshotID = "latest"
		}
		if _, err := exec.LookPath("restic"); err != nil {
			writeRestoreError(w, http.StatusInternalServerError, "RESTORE_PLAN_PREREQ_FAILED", "restic not found on PATH")
			return
		}
		if _, err := resticEnvForUI(cfgPath); err != nil {
			logRestoreDetail("RESTORE_PLAN_PREREQ_FAILED", err)
			writeRestoreError(w, http.StatusInternalServerError, "RESTORE_PLAN_PREREQ_FAILED", "restore prerequisites unavailable")
			return
		}
		validateErrs, confirmRequired := validateRestoreRequest(req)
		writeJSON(w, map[string]interface{}{
			"ok":               len(validateErrs) == 0,
			"confirm_required": confirmRequired,
			"errors":           validateErrs,
			"request":          req,
		})
	}
}

func (s *Server) handleRestoreRun(cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req restoreRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeRestoreError(w, http.StatusBadRequest, "RESTORE_BAD_REQUEST", "invalid JSON request")
			return
		}
		if req.SnapshotID == "" {
			req.SnapshotID = "latest"
		}
		validateErrs, confirmRequired := validateRestoreRequest(req)
		if len(validateErrs) > 0 {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, map[string]interface{}{"ok": false, "errors": validateErrs})
			return
		}
		if confirmRequired && !req.ConfirmOverwrite {
			w.WriteHeader(http.StatusConflict)
			writeJSON(w, map[string]interface{}{
				"ok":               false,
				"confirm_required": true,
				"errors":           []string{"target exists and is non-empty; set confirm_overwrite=true to continue"},
			})
			return
		}

		env, err := resticEnvForUI(cfgPath)
		if err != nil {
			logRestoreDetail("RESTORE_ENV_FAILED", err)
			writeRestoreError(w, http.StatusInternalServerError, "RESTORE_ENV_FAILED", "restore environment unavailable")
			return
		}
		start := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), restoreTimeout)
		defer cancel()
		switch req.Type {
		case "file":
			if err := os.MkdirAll(filepath.Dir(req.Target), 0o700); err != nil {
				logRestoreDetail("RESTORE_TARGET_DIR_FAILED", err)
				writeRestoreError(w, http.StatusInternalServerError, "RESTORE_TARGET_DIR_FAILED", "failed to prepare target directory")
				return
			}
			tmp, err := os.CreateTemp(filepath.Dir(req.Target), ".xentz-restore-*")
			if err != nil {
				logRestoreDetail("RESTORE_OUTPUT_OPEN_FAILED", err)
				writeRestoreError(w, http.StatusInternalServerError, "RESTORE_OUTPUT_OPEN_FAILED", "failed to prepare restore output")
				return
			}
			tmpName := tmp.Name()
			defer func() {
				_ = tmp.Close()
				_ = os.Remove(tmpName)
			}()
			cmd := exec.CommandContext(ctx, "restic", "dump", req.SnapshotID, req.Path)
			cmd.Env = env
			var stderr bytes.Buffer
			cmd.Stdout = tmp
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				logRestoreDetail("RESTORE_EXEC_FAILED", fmt.Errorf("%w (%s)", err, tailText(stderr.String(), 2048)))
				writeRestoreError(w, http.StatusInternalServerError, "RESTORE_EXEC_FAILED", "restore failed")
				return
			}
			if err := tmp.Sync(); err != nil {
				logRestoreDetail("RESTORE_OUTPUT_SYNC_FAILED", err)
				writeRestoreError(w, http.StatusInternalServerError, "RESTORE_OUTPUT_SYNC_FAILED", "failed to finalize restored file")
				return
			}
			if err := tmp.Close(); err != nil {
				logRestoreDetail("RESTORE_OUTPUT_CLOSE_FAILED", err)
				writeRestoreError(w, http.StatusInternalServerError, "RESTORE_OUTPUT_CLOSE_FAILED", "failed to finalize restored file")
				return
			}
			if err := os.Rename(tmpName, req.Target); err != nil {
				logRestoreDetail("RESTORE_OUTPUT_RENAME_FAILED", err)
				writeRestoreError(w, http.StatusInternalServerError, "RESTORE_OUTPUT_RENAME_FAILED", "failed to place restored file")
				return
			}
		case "folder", "snapshot":
			if err := os.MkdirAll(req.Target, 0o700); err != nil {
				logRestoreDetail("RESTORE_TARGET_DIR_FAILED", err)
				writeRestoreError(w, http.StatusInternalServerError, "RESTORE_TARGET_DIR_FAILED", "failed to prepare target directory")
				return
			}
			args := []string{"restore", req.SnapshotID, "--target", req.Target}
			if req.Type == "folder" {
				args = append(args, "--include", req.Path)
			}
			cmd := exec.CommandContext(ctx, "restic", args...)
			cmd.Env = env
			var stderr bytes.Buffer
			cmd.Stdout = io.Discard
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				logRestoreDetail("RESTORE_EXEC_FAILED", fmt.Errorf("%w (%s)", err, tailText(stderr.String(), 2048)))
				writeRestoreError(w, http.StatusInternalServerError, "RESTORE_EXEC_FAILED", "restore failed")
				return
			}
		default:
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, map[string]interface{}{"ok": false, "errors": []string{"type must be file, folder, or snapshot"}})
			return
		}

		writeJSON(w, map[string]interface{}{
			"ok":          true,
			"type":        req.Type,
			"snapshot_id": req.SnapshotID,
			"path":        req.Path,
			"target":      req.Target,
			"duration_ms": time.Since(start).Milliseconds(),
			"open_hint":   openHint(req.Target),
		})
	}
}

func resticEnvForUI(cfgPath string) ([]string, error) {
	cfg, err := config.Read(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if cfg.Restic.Repository == "" {
		return nil, fmt.Errorf("restic.repository is empty in config")
	}
	var pw string
	if cfg.Restic.PasswordFile == "" {
		pw, err = config.GetResticPassword(cfg)
		if err != nil {
			return nil, fmt.Errorf("resolve restic password: %w", err)
		}
	}
	return append(os.Environ(), backup.ResticEnv(cfg, pw)...), nil
}

func validateRestoreRequest(req restoreRequest) ([]string, bool) {
	var errs []string
	confirmRequired := false
	if req.Type != "file" && req.Type != "folder" && req.Type != "snapshot" {
		errs = append(errs, "type must be file, folder, or snapshot")
	}
	if strings.TrimSpace(req.Target) == "" {
		errs = append(errs, "target is required")
	} else if !filepath.IsAbs(req.Target) {
		errs = append(errs, "target must be an absolute path")
	}
	if req.Type == "file" || req.Type == "folder" {
		if strings.TrimSpace(req.Path) == "" {
			errs = append(errs, "path is required for file/folder restore")
		}
	}
	if isDangerousTarget(req.Target) {
		errs = append(errs, "target path is unsafe")
	}
	if st, err := os.Stat(req.Target); err == nil {
		if req.Type == "file" && st.IsDir() {
			errs = append(errs, "file restore target cannot be an existing directory")
		}
		if (req.Type == "folder" || req.Type == "snapshot") && !st.IsDir() {
			errs = append(errs, "folder/snapshot restore target cannot be an existing file")
		}
		if req.Type == "file" && !st.IsDir() {
			confirmRequired = true
		}
		if (req.Type == "folder" || req.Type == "snapshot") && st.IsDir() {
			if hasEntries, e := dirHasEntries(req.Target); e == nil && hasEntries {
				confirmRequired = true
			}
		}
	}
	return errs, confirmRequired
}

func isDangerousTarget(target string) bool {
	clean := filepath.Clean(target)
	return clean == "/" || clean == "/System" || clean == "/Library" || clean == "/usr"
}

func dirHasEntries(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func tailText(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}

func openHint(target string) string {
	if runtime.GOOS == "darwin" {
		return "open \"" + target + "\""
	}
	return ""
}

func isSubPath(path, root string) bool {
	p := filepath.Clean(path)
	r := filepath.Clean(root)
	if p == r {
		return true
	}
	rel, err := filepath.Rel(r, p)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func writeRestoreError(w http.ResponseWriter, status int, code, message string) {
	w.WriteHeader(status)
	writeJSON(w, map[string]interface{}{
		"ok":      false,
		"code":    code,
		"message": message,
	})
}

func logRestoreDetail(code string, err error) {
	if err == nil {
		return
	}
	fmt.Printf("local-ui %s: %v\n", code, err)
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
