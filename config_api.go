package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Settings endpoints for the file-backed configuration.
//
// These exist so a user never has to leave the dashboard to find, back up, or
// restore their nodes and AI providers. The directory is never taken from the
// request: it is always resolved from configDir(). Accepting a caller-supplied
// path would turn "open the config folder" into an arbitrary program execution
// primitive, since the handler hands the path to a shell command.
//
// The dashboard can be reached two ways, and the settings panel adapts:
//
//   - From the machine running the service (the browser sent the request over
//     loopback). The configuration files are the user's own, so the panel
//     offers an "open containing folder" link, plus export and import.
//   - From anywhere else. The caller is a visitor to someone else's
//     deployment; the files sit on the server's disk and are not reachable
//     from their browser. The panel offers none of that.
//
// The response deliberately carries no filesystem paths at all. The UI never
// renders one: the presence of the reveal link is the only cue the user gets
// about which backend is in play, so there is no reason to send the server's
// directory layout anywhere.
//
// Note this is about reachability of the file operations, not about where the
// configuration lives: the dashboard's own node/model data always comes from
// this service's configuration.
//
// Locality is decided from the request's source address rather than from the
// URL the browser used. The hostname is attacker-controlled and a reverse proxy
// rewrites it, so it cannot be trusted; the connection's peer address can.
// X-Forwarded-For is deliberately ignored, since a client can set it freely.

// configInfoResponse describes the configuration backend for the settings UI.
type configInfoResponse struct {
	// LocalMode reports whether this caller can reach the configuration files.
	LocalMode bool `json:"localMode"`
	// CanReveal reports whether opening a file browser is meaningful: the
	// caller must be local and the platform must have a desktop.
	CanReveal     bool `json:"canReveal"`
	NodesExists   bool `json:"nodesExists"`
	ModelsExist   bool `json:"modelsExist"`
	HasAdminToken bool `json:"hasAdminToken"`
	NodesCount    int  `json:"nodesCount"`
	ModelsCount   int  `json:"modelsCount"`
}

func (s *server) handleConfigInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	nodePath, modelPath := nodesFilePath(), modelsFilePath()

	nodesCount, modelsCount := 0, 0
	if s.store != nil {
		if records, err := s.store.listNodes(); err == nil {
			nodesCount = len(records)
		}
		if records, err := s.store.listModels(); err == nil {
			modelsCount = len(records)
		}
	}

	local := isLoopbackRequest(r)
	writeJSON(w, http.StatusOK, configInfoResponse{
		LocalMode:     local,
		CanReveal:     local && hasDesktopSession(),
		NodesExists:   fileExists(nodePath),
		ModelsExist:   fileExists(modelPath),
		HasAdminToken: s.currentAdminToken() != "",
		NodesCount:    nodesCount,
		ModelsCount:   modelsCount,
	})
}

// isLoopbackRequest reports whether the request reached us over the loopback
// interface, which means the browser is on the same machine as the service.
//
// IPv6-mapped IPv4 addresses ("::ffff:127.0.0.1") are unwrapped, since Go
// reports them in that form when a listener accepts both families.
func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	// "::ffff:127.0.0.1" carries the IPv4 address after the last colon.
	if index := strings.LastIndex(host, ":"); index >= 0 && strings.Contains(host, ".") {
		host = host[index+1:]
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// hasDesktopSession reports whether a file manager could be launched.
//
// This is separate from isLoopbackRequest: a headless Linux box reached from
// its own console is local, but has no desktop to open a window on, and
// showing a button that always fails is worse than hiding it.
func hasDesktopSession() bool {
	switch runtime.GOOS {
	case "windows", "darwin":
		return true
	default:
		return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// requireLocalOwner rejects a configuration change that did not originate on
// the machine running the service.
//
// The settings panel hides these controls for a remote visitor, but hiding is
// presentation, not enforcement: the endpoint is reachable by anyone who knows
// the URL. The check belongs here. It delegates to requireFileBackend so the
// node, AI-profile and configuration endpoints all agree on one rule.
func requireLocalOwner(w http.ResponseWriter, r *http.Request) bool {
	return requireFileBackend(w, r)
}

// handleConfigReveal opens the configuration directory in the OS file manager.
//
// The path is fixed (configDir) and the action requires the admin token, so the
// worst case for an authenticated caller is "a folder opens on the server".
func (s *server) handleConfigReveal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if token := s.currentAdminToken(); token != "" && !authorizedAdminRequest(r, token) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "admin token required"})
		return
	}
	// Opening a window only affects the machine running the service, so it is
	// meaningful solely for someone sitting at that machine.
	if !requireLocalOwner(w, r) {
		return
	}
	if !hasDesktopSession() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "当前系统未检测到桌面环境，无法打开文件管理器"})
		return
	}
	dir := configDir()
	// Create the directory first so the button works before anything has been
	// saved; otherwise the file manager would open nothing.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("无法创建配置目录: %v", err)})
		return
	}
	if err := openInFileManager(dir); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("无法打开目录: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"opened": true})
}

// openInFileManager launches the platform file browser for dir.
//
// Each branch passes the path as a separate argument rather than building a
// shell string, so a path containing spaces or shell metacharacters cannot be
// interpreted as a command.
func openInFileManager(dir string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "windows":
		// explorer.exe returns a non-zero exit code even on success, so its
		// exit status must not be treated as a failure.
		command, args = "explorer", []string{dir}
	case "darwin":
		command, args = "open", []string{dir}
	default:
		command, args = "xdg-open", []string{dir}
	}
	cmd := exec.Command(command, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reap the child in the background: explorer/xdg-open can linger, and
	// waiting here would block the HTTP handler.
	go func() { _ = cmd.Wait() }()
	return nil
}

// configExportResponse is the portable backup format. It carries a version so
// a future format change can be rejected rather than silently mis-parsed.
type configExportResponse struct {
	Version   int           `json:"version"`
	Exported  string        `json:"exportedAt"`
	Nodes     []storedNode  `json:"nodes"`
	Providers []storedModel `json:"providers"`
}

// handleConfigExport streams the current configuration as a JSON download.
func (s *server) handleConfigExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if token := s.currentAdminToken(); token != "" && !authorizedAdminRequest(r, token) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "admin token required"})
		return
	}
	// The export contains plaintext API keys, so it is restricted to the
	// machine that owns them.
	if !requireLocalOwner(w, r) {
		return
	}
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "配置存储不可用"})
		return
	}
	nodes, err := s.store.listNodes()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("导出节点失败: %v", err)})
		return
	}
	providers, err := s.store.listModels()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("导出模型配置失败: %v", err)})
		return
	}
	payload := configExportResponse{
		Version:   1,
		Exported:  time.Now().Format(time.RFC3339),
		Nodes:     nodes,
		Providers: providers,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="log-agent-config.json"`)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(payload)
}

// handleConfigImport replaces the configuration wholesale.
//
// Validation happens before anything is written, so a malformed file fails as
// a whole rather than leaving a half-applied configuration behind.
func (s *server) handleConfigImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if token := s.currentAdminToken(); token != "" && !authorizedAdminRequest(r, token) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "admin token required"})
		return
	}
	// An import replaces the whole configuration, so it is restricted to the
	// machine that owns the files.
	if !requireLocalOwner(w, r) {
		return
	}
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "配置存储不可用"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)

	var payload struct {
		Version   int           `json:"version"`
		Nodes     []storedNode  `json:"nodes"`
		Providers []storedModel `json:"providers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "配置文件格式不正确"})
		return
	}
	if payload.Version != 0 && payload.Version != 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("不支持的配置版本: %d", payload.Version)})
		return
	}
	if len(payload.Nodes) == 0 && len(payload.Providers) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "配置文件中没有可导入的节点或模型"})
		return
	}
	for _, node := range payload.Nodes {
		if strings.TrimSpace(node.Name) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "存在缺少名称的节点"})
			return
		}
		if _, _, err := parseDozzleURL(strings.TrimSpace(node.Address)); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("节点 %q 地址无效: %v", node.Name, err)})
			return
		}
	}
	if err := s.store.replaceAll(payload.Nodes, payload.Providers); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("导入失败: %v", err)})
		return
	}
	// Rebuild the live node list from the new file so the change takes effect
	// without a restart.
	if err := s.reloadNodesFromStore(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("导入成功但重新加载节点失败: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"imported":  true,
		"nodes":     len(payload.Nodes),
		"providers": len(payload.Providers),
	})
}

// reloadNodesFromStore tears down the existing node connections and rebuilds
// them from the configuration file.
func (s *server) reloadNodesFromStore() error {
	s.mu.Lock()
	for id, cancel := range s.nodeCancels {
		if cancel != nil {
			cancel()
		}
		delete(s.nodeCancels, id)
		delete(s.nodeContexts, id)
	}
	s.containerNames = make(map[string]map[string]string)
	s.containerLogs = make(map[string][]LogEntry)
	s.historyCoverage = make(map[string]time.Time)
	s.historyGeneration++
	s.nodes = nil
	s.mu.Unlock()

	if err := s.loadNodesFromStore(); err != nil {
		return err
	}
	s.startPersistedNodes()
	return nil
}
