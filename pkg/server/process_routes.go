package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/server/pages"
	"github.com/OhRats-Technologies/rc/pkg/version"
)

func (s *Server) registerProcessAPIRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/devices/{id}/processes", s.allowSessionOrAPI(http.HandlerFunc(s.handleDeviceProcesses)))
	mux.Handle("POST /api/v1/devices/{id}/processes", s.allowSessionOrAPI(http.HandlerFunc(s.handleDeviceProcesses)))
	mux.Handle("GET /api/v1/processes/{id}", s.allowSessionOrAPI(http.HandlerFunc(s.handleProcessDetail)))
	mux.HandleFunc("GET /devices/{id}/processes/{process}", s.handleProcessPage)
}

func (s *Server) handleDeviceProcesses(w http.ResponseWriter, req *http.Request) {
	userID, ok := requestUserID(req)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	deviceID := req.PathValue("id")
	device, ok := s.registry.Get(deviceID)
	if !ok {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	role, ok := s.registry.requestRole(req, device.WorkspaceID)
	if !ok {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	if role != "owner" && role != "operator" {
		writeError(w, http.StatusForbidden, "operator required")
		return
	}
	if req.Method == http.MethodGet {
		processes := s.processes.forDevice(deviceID)
		items := make([]map[string]any, 0, len(processes))
		for _, process := range processes {
			items = append(items, processJSON(process))
		}
		writeJSON(w, http.StatusOK, map[string]any{"processes": items})
		return
	}
	if req.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.nodes.Online(deviceID) {
		writeError(w, http.StatusConflict, "device is offline")
		return
	}
	input := struct {
		Terminal bool `json:"terminal"`
	}{}
	if req.ContentLength > 0 {
		if err := json.NewDecoder(io.LimitReader(req.Body, 8<<10)).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request")
			return
		}
	}
	user, _ := s.auth.GetUser(userID)
	name := ""
	if user != nil {
		name = user.Name
	}
	origin := "browser"
	if apiPrincipalFrom(req) != nil {
		origin = "api"
	}
	id := "proc_" + randomID(18)
	s.processes.add(browserProcess{
		ID: id, DeviceID: deviceID, UserID: userID, UserName: name,
		Origin: origin, Terminal: input.Terminal, Status: "starting",
		CreatedAt: time.Now().UTC(),
	})
	s.events.Publish("process.created", map[string]any{
		"deviceId": deviceID, "processId": id, "origin": origin, "terminal": input.Terminal,
	})
	writeJSON(w, http.StatusCreated, map[string]any{"processId": id})
}

func (s *Server) handleProcessDetail(w http.ResponseWriter, req *http.Request) {
	if _, ok := requestUserID(req); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	process, ok := s.processes.get(req.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "process not found")
		return
	}
	device, ok := s.registry.Get(process.DeviceID)
	if !ok {
		writeError(w, http.StatusNotFound, "process not found")
		return
	}
	role, ok := s.registry.requestRole(req, device.WorkspaceID)
	if !ok || (role != "owner" && role != "operator") {
		writeError(w, http.StatusNotFound, "process not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"process": processJSON(process)})
}

func (s *Server) handleProcessPage(w http.ResponseWriter, req *http.Request) {
	user, _, err := s.currentSession(req)
	if err != nil {
		http.Redirect(w, req, "/login?next="+escape(req.URL.Path), http.StatusSeeOther)
		return
	}
	device, ok := s.registry.Get(req.PathValue("id"))
	if !ok {
		http.NotFound(w, req)
		return
	}
	process, ok := s.processes.get(req.PathValue("process"))
	if !ok || process.DeviceID != device.ID {
		http.NotFound(w, req)
		return
	}
	role, err := s.auth.WorkspaceRole(user.ID, device.WorkspaceID)
	if err != nil {
		http.NotFound(w, req)
		return
	}
	controllable := role == "owner" || (role == "operator" && process.UserID == user.ID)
	running := process.Status == "starting" || process.Status == "running"
	interactive := process.Terminal && running && controllable
	message := process.Error
	if message == "" && running && !controllable {
		message = "Live control belongs to " + process.UserName + "."
	}
	body := processPageBody(device, process, process.UserName, interactive, running, message)
	sidebar := s.renderSidebar(user.ID, user.Name, req.URL.Path)
	s.writePage(w, http.StatusOK, pages.Authenticated(process.ID, sidebar, body,
		[]string{"process-terminal"}, version.Version), "no-store")
}

func processPageBody(device *RegisteredDevice, process browserProcess, userName string, interactive, running bool, message string) string {
	terminalBody := `<pre class="terminal-transcript">Live process output is not available from this page.</pre>`
	actions := ""
	if interactive {
		terminalBody = `<pre id="process-transcript" class="terminal-transcript" hidden></pre>` +
			`<div id="terminal-host" class="terminal-host" hidden></div>` +
			`<div class="mobile-terminal-keys" aria-label="Terminal keys">` + mobileTerminalKeys() + `</div>`
		actions = `<div id="terminal-actions" class="terminal-actions">` +
			`<button class="text-button" data-signal="INT" type="button">CTRL-C</button>` +
			`<button class="text-button" data-signal="TERM" type="button">TERM</button>` +
			`<button class="text-button" data-signal="KILL" type="button">KILL</button></div>`
	}
	return `<div class="page process-page" data-process-page="` + escape(process.ID) +
		`" data-device-id="` + escape(device.ID) + `" data-process-status="` + escape(process.Status) +
		`" data-process-live="true" data-process-interactive="` + boolString(interactive) + `">` +
		`<header class="page-header process-header"><div><p class="eyebrow"><a href="/devices/` +
		escape(device.ID) + `">` + escape(strings.ToUpper(device.Name)) + `</a> / PROCESS</p>` +
		`<h1 class="mono process-title">` + escape(process.ID) + `</h1><p class="meta">` +
		escape(strings.ToUpper(process.Origin)) + ` · STARTED BY ` + escape(userName) + `</p></div>` +
		`<span id="process-state" class="status` + onlineClass(running) + `">` +
		escape(strings.ToUpper(process.Status)) + `</span></header>` +
		`<div id="terminal-toolbar" class="terminal-toolbar"><span class="terminal-label">` +
		boolChoice(process.Terminal, "PTY", "PROCESS") + ` · ` +
		escape(shortProcess(process.ID)) + `</span>` +
		boolChoice(interactive, ` · <span id="control-transport">CONNECTING</span>`, "") + `</div>` +
		actions + terminalBody +
		`<div id="process-client-alert" class="process-client-alert" hidden>` +
		`<p id="process-client-error" class="error" role="alert" aria-live="polite"></p>` +
		`<button class="text-button" type="button" data-process-error-dismiss>DISMISS</button></div>` +
		`<p id="process-message" class="meta process-message">` + escape(message) + `</p></div>`
}

func mobileTerminalKeys() string {
	keys := map[string]string{
		"ESC": "ESC", "CTRL": "CTRL", "ALT": "ALT", "TAB": "TAB",
		"LEFT": "←", "UP": "↑", "DOWN": "↓", "RIGHT": "→",
	}
	order := []string{"ESC", "CTRL", "ALT", "TAB", "LEFT", "UP", "DOWN", "RIGHT"}
	var out strings.Builder
	for _, key := range order {
		out.WriteString(`<button type="button" data-terminal-key="` + key + `">` + keys[key] + `</button>`)
	}
	return out.String()
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func boolChoice(value bool, yes, no string) string {
	if value {
		return yes
	}
	return no
}

func shortProcess(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
