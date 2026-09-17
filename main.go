package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"html"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// The frontend is embedded so the whole dashboard can be shipped as one Go binary.
//
//go:embed index.html styles.css app.js assistant-ui.js bootstrap.js favicon.png ai-icon.png loading.gif
var frontend embed.FS

const (
	defaultMaxStoredLogs = 100000
	maxAllowedStoredLogs = 1000000
	maxContainerLogs     = 5000
)

var maxStoredLogs = defaultMaxStoredLogs

const (
	dozzleHistoryPageSize = 500
	historyFetchTimeout   = 90 * time.Second
)

type Node struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	URL        string          `json:"url"`
	Style      string          `json:"style"`
	Count      int             `json:"count"`
	Initial    string          `json:"initial"`
	Warning    bool            `json:"warning"`
	Status     string          `json:"status"`
	Error      string          `json:"error,omitempty"`
	Latency    int64           `json:"latency"`
	Version    string          `json:"version"`
	Containers []containerInfo `json:"containers"`

	baseURL     string
	hostID      string
	containerID string
	dbID        int64
}

type containerInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

type commandConnectionRequest struct {
	Host        string `json:"host"`
	Port        string `json:"port"`
	User        string `json:"user"`
	Auth        string `json:"auth"`
	Secret      string `json:"secret"`
	Fingerprint string `json:"fingerprint"`
}

type commandExecutionRequest struct {
	Host        string `json:"host"`
	Port        string `json:"port"`
	User        string `json:"user"`
	Auth        string `json:"auth"`
	Secret      string `json:"secret"`
	Fingerprint string `json:"fingerprint"`
	Command     string `json:"command"`
}

type commandHostKeyError struct{ Fingerprint string }

func (e *commandHostKeyError) Error() string { return "需要确认服务器指纹" }

var commandConnectionDial = dialCommandSSH
var commandExecutionRun = runCommandSSH

type LogEntry struct {
	ID        int64  `json:"id"`
	Date      string `json:"date"`
	Time      string `json:"time"`
	Timestamp int64  `json:"timestamp"`
	Level     string `json:"level"`
	Node      string `json:"node"`
	Container string `json:"container"`
	Message   string `json:"message"`
	nodeID    string
	remoteID  uint32 `json:"-"`
}

type nodeRequest struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Style string `json:"style"`
}

type ruleUpdateRequest struct {
	Rule    string          `json:"rule"`
	Enabled *bool           `json:"enabled"`
	Rules   map[string]bool `json:"rules"`
	Order   []string        `json:"order"`
}

type aiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aiProfile struct {
	ProfileID int64  `json:"profile_id,omitempty"`
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	Model     string `json:"model"`
	Type      string `json:"type"`
}

type aiChatRequest struct {
	Messages    []aiMessage    `json:"messages"`
	Logs        []LogEntry     `json:"logs"`
	Attachments []aiAttachment `json:"attachments,omitempty"`
	Config      *aiProfile     `json:"config,omitempty"`
}

type aiAttachment struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Data string `json:"data"`
}

type aiProviderMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// configBackend selects where the dashboard keeps nodes and AI providers.
//
// There are two ways to reach this dashboard, and they must not interfere:
//
//   - backendFile: the page was opened on the machine running the service
//     (over loopback). The configuration is genuinely the user's own, so it is
//     read from and written to nodes.json / models.json, and the file can be
//     opened in a file manager.
//   - backendBrowser: the page was opened from anywhere else, so the caller is
//     a visitor on someone else's deployment. Their nodes and API keys belong
//     to them and must never be written to the server's disk, so everything
//     stays in their browser's local storage.
//
// The backend is derived per request from the source address, never chosen by
// the user and never persisted: the same service is legitimately both things
// to two different callers at the same time.
type configBackend string

const (
	backendBrowser configBackend = "browser"
	backendFile    configBackend = "file"
)

// backendForRequest reports which storage the caller should use.
func backendForRequest(r *http.Request) configBackend {
	if isLoopbackRequest(r) {
		return backendFile
	}
	return backendBrowser
}

type appSettings struct {
	Environment string `json:"environment"`
	AdminToken  string `json:"admin_token,omitempty"`
}

type settingsResponse struct {
	Environment          string `json:"environment"`
	AdminTokenConfigured bool   `json:"adminTokenConfigured"`
}

type aiChatResponse struct {
	Choices []struct {
		Message aiMessage `json:"message"`
	} `json:"choices"`
}

type server struct {
	mu                    sync.RWMutex
	store                 *configStore
	nodes                 []Node
	logs                  []LogEntry
	processed             int
	nextLogID             int64
	evictedLogs           int64
	lastEvictedAt         int64
	historyRange          string
	historyNodeScope      string
	historyGeneration     uint64
	historyPending        int
	rules                 map[string]bool
	ruleOrder             []string
	subscribers           map[chan LogEntry]struct{}
	containerNames        map[string]map[string]string
	containerLogs         map[string][]LogEntry
	historyCoverage       map[string]time.Time
	historyLoads          map[string]struct{}
	historyLoadGeneration map[string]uint64
	streams               map[string]struct{}
	nodeContexts          map[string]context.Context
	nodeCancels           map[string]context.CancelFunc
	settings              appSettings
}

type dozzleConfig struct {
	Version string       `json:"version"`
	Hosts   []dozzleHost `json:"hosts"`
}

type dozzleHost struct {
	ID        string `json:"id"`
	Available bool   `json:"available"`
}

type dozzleContainer struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
	Host  string `json:"host"`
}

type dozzleLogEvent struct {
	Type      string          `json:"t"`
	Message   json.RawMessage `json:"m"`
	Raw       string          `json:"rm"`
	Timestamp int64           `json:"ts"`
	ID        uint32          `json:"id"`
	Level     string          `json:"l"`
	Container string          `json:"c"`
}

type dozzleLogLine struct {
	Message string `json:"m"`
}

type bootstrapResponse struct {
	Nodes          []Node          `json:"nodes"`
	Logs           []LogEntry      `json:"logs"`
	Processed      int             `json:"processed"`
	Range          string          `json:"range"`
	HistoryLoading bool            `json:"historyLoading"`
	Rules          map[string]bool `json:"rules"`
	RuleOrder      []string        `json:"ruleOrder"`
	Storage        storageStats    `json:"storage"`
	// StorageMode tells the client whether to keep nodes and AI providers here
	// or in its own local storage.
	StorageMode string `json:"storageMode"`
}

type containerLogsResponse struct {
	Logs    []LogEntry `json:"logs"`
	Loading bool       `json:"loading,omitempty"`
}

type containerHistoryPageResponse struct {
	Logs       []LogEntry `json:"logs"`
	HasMore    bool       `json:"hasMore"`
	NextBefore int64      `json:"nextBefore,omitempty"`
}

type containerHistorySearchResponse struct {
	Logs []LogEntry `json:"logs"`
}

type storageStats struct {
	Used          int   `json:"used"`
	Capacity      int   `json:"capacity"`
	Percent       int   `json:"percent"`
	Evicted       int64 `json:"evicted"`
	LastEvictedAt int64 `json:"lastEvictedAt"`
}

func main() {
	s := newServer()
	address := listenAddress()
	log.Printf("Log Agent is running at http://localhost:%s", strings.TrimPrefix(address, ":"))
	log.Fatal(http.ListenAndServe(address, newHTTPHandler(s)))
}

// listenAddress resolves the listen address. PORT (or LOG_AGENT_PORT) lets a
// user run a second instance next to a Dozzle install that already owns 8099.
func listenAddress() string {
	for _, name := range []string{"LOG_AGENT_PORT", "PORT"} {
		raw := strings.TrimSpace(os.Getenv(name))
		if raw == "" {
			continue
		}
		raw = strings.TrimPrefix(raw, ":")
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			log.Printf("ignoring invalid %s=%q; falling back to 8099", name, raw)
			continue
		}
		return ":" + strconv.Itoa(port)
	}
	return ":8099"
}

func newHTTPHandler(s *server) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/bootstrap", s.handleBootstrap)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/logs/container", s.handleContainerLogs)
	mux.HandleFunc("/api/logs/container/page", s.handleContainerHistoryPage)
	mux.HandleFunc("/api/logs/container/search", s.handleContainerHistorySearch)
	mux.HandleFunc("/api/logs/range", s.handleLogRange)
	mux.HandleFunc("/api/logs/cache/clear", s.handleClearLogCache)
	mux.HandleFunc("/api/rules", s.handleRules)
	mux.HandleFunc("/api/ai/status", s.handleAIStatus)
	mux.HandleFunc("/api/ai/chat", s.handleAIChat)
	mux.HandleFunc("/api/ai/profiles", s.handleAIProfiles)
	mux.HandleFunc("/api/commands/test", s.handleCommandConnectionTest)
	mux.HandleFunc("/api/commands/upload", s.handleCommandFileUpload)
	mux.HandleFunc("/api/commands/execute", s.handleCommandExecute)
	mux.HandleFunc("/api/settings", s.handleSettings)
	mux.HandleFunc("/api/settings/admin", s.handleAdminSettings)
	mux.HandleFunc("/api/settings/environment", s.handleEnvironmentSettings)
	mux.HandleFunc("/api/config/info", s.handleConfigInfo)
	mux.HandleFunc("/api/config/reveal", s.handleConfigReveal)
	mux.HandleFunc("/api/config/export", s.handleConfigExport)
	mux.HandleFunc("/api/config/import", s.handleConfigImport)
	mux.HandleFunc("/api/nodes", s.handleNodes)
	mux.HandleFunc("/api/nodes/", s.handleNode)
	mux.HandleFunc("/api/stream", s.handleStream)

	static, err := fs.Sub(frontend, ".")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(static)))
	return withSecurityHeaders(mux)
}

func (s *server) handleCommandConnectionTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var request commandConnectionRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "连接参数无效"})
		return
	}
	host := strings.TrimSpace(request.Host)
	port := strings.TrimSpace(request.Port)
	if host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请填写服务器地址"})
		return
	}
	if port == "" {
		port = "22"
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "服务器端口无效"})
		return
	}
	request.Host = host
	request.Port = strconv.Itoa(portNumber)
	request.User = strings.TrimSpace(request.User)
	request.Auth = strings.TrimSpace(request.Auth)
	request.Fingerprint = strings.TrimSpace(request.Fingerprint)
	if request.User == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请填写用户名"})
		return
	}
	if strings.TrimSpace(request.Secret) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请选择密钥文件或填写密码"})
		return
	}
	if err := commandConnectionDial(request); err != nil {
		var hostKeyError *commandHostKeyError
		if errors.As(err, &hostKeyError) {
			writeJSON(w, http.StatusPreconditionRequired, map[string]string{"error": hostKeyError.Error(), "fingerprint": hostKeyError.Fingerprint})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "连接失败：" + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func dialCommandSSH(request commandConnectionRequest) error {
	client, err := newCommandSSHClient(request)
	if err != nil {
		return err
	}
	return client.Close()
}

func newCommandSSHClient(request commandConnectionRequest) (*ssh.Client, error) {
	var authMethod ssh.AuthMethod
	if request.Auth == "password" {
		authMethod = ssh.Password(request.Secret)
	} else {
		signer, err := ssh.ParsePrivateKey([]byte(request.Secret))
		if err != nil {
			return nil, fmt.Errorf("密钥文件无效：%w", err)
		}
		authMethod = ssh.PublicKeys(signer)
	}
	hostKeyCallback := func(_ string, _ net.Addr, key ssh.PublicKey) error {
		fingerprint := ssh.FingerprintSHA256(key)
		if request.Fingerprint == "" {
			return &commandHostKeyError{Fingerprint: fingerprint}
		}
		if subtle.ConstantTimeCompare([]byte(request.Fingerprint), []byte(fingerprint)) != 1 {
			return fmt.Errorf("服务器指纹不匹配，当前为 %s", fingerprint)
		}
		return nil
	}
	return ssh.Dial("tcp", net.JoinHostPort(request.Host, request.Port), &ssh.ClientConfig{
		User: request.User, Auth: []ssh.AuthMethod{authMethod}, HostKeyCallback: hostKeyCallback, Timeout: 5 * time.Second,
	})
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func (s *server) handleCommandFileUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "读取上传文件失败：" + err.Error()})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请选择要上传的文件"})
		return
	}
	defer file.Close()
	request := commandConnectionRequest{Host: strings.TrimSpace(r.FormValue("host")), Port: strings.TrimSpace(r.FormValue("port")), User: strings.TrimSpace(r.FormValue("user")), Auth: strings.TrimSpace(r.FormValue("auth")), Secret: r.FormValue("secret"), Fingerprint: strings.TrimSpace(r.FormValue("fingerprint"))}
	if request.Port == "" {
		request.Port = "22"
	}
	if request.Host == "" || request.User == "" || request.Secret == "" || request.Fingerprint == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "服务器连接信息不完整，请重新测试连接"})
		return
	}
	filename := path.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
	if filename == "." || filename == "/" || filename == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "文件名无效"})
		return
	}
	homeDirectory := path.Join("/home", request.User)
	if request.User == "root" {
		homeDirectory = "/root"
	}
	destination := strings.TrimSpace(r.FormValue("destination"))
	if destination == "" {
		destination = homeDirectory
	}
	if strings.ContainsRune(destination, '\x00') {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "服务器位置无效"})
		return
	}
	destination = path.Clean(destination)
	if destination != homeDirectory && !strings.HasPrefix(destination, homeDirectory+"/") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "服务器位置必须位于 " + homeDirectory + " 内"})
		return
	}
	remotePath := path.Join(destination, filename)
	client, err := newCommandSSHClient(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "SSH 连接失败：" + err.Error()})
		return
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "创建 SSH 会话失败：" + err.Error()})
		return
	}
	defer session.Close()
	session.Stdin = file
	if err := session.Run("umask 077 && mkdir -p -- " + shellQuote(destination) + " && cat > " + shellQuote(remotePath)); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "写入远程文件失败：" + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": remotePath})
}

func splitCommandWords(command string) ([]string, error) {
	var words []string
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			words = append(words, current.String())
			current.Reset()
		}
	}
	for _, char := range command {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				current.WriteRune(char)
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		if char == ' ' || char == '\t' {
			flush()
			continue
		}
		current.WriteRune(char)
	}
	if escaped || quote != 0 {
		return nil, errors.New("指令中的引号或转义不完整")
	}
	flush()
	return words, nil
}

func commandHomeDirectory(user string) string {
	if user == "root" {
		return "/root"
	}
	return path.Join("/home", user)
}
func commandPathInsideHome(value, home string) bool {
	if value == "" || strings.HasPrefix(value, "-") {
		return true
	}
	candidate := value
	if !strings.HasPrefix(candidate, "/") {
		candidate = path.Join(home, candidate)
	}
	candidate = path.Clean(candidate)
	return candidate == home || strings.HasPrefix(candidate, home+"/")
}

func validateRemoteCommand(command, user string) error {
	if strings.ContainsAny(command, "`$;&|<>\r\n") {
		return errors.New("不允许使用重定向、管道、命令拼接或变量展开")
	}
	words, err := splitCommandWords(command)
	if err != nil {
		return err
	}
	if len(words) == 0 {
		return errors.New("指令不能为空")
	}
	if (words[0] == "systemctl" && len(words) == 3 && words[1] == "stop") || (words[0] == "docker" && len(words) == 3 && words[1] == "stop") {
		return nil
	}
	if len(words) == 5 && words[0] == "sudo" && words[1] == "-n" && words[2] == "systemctl" && words[3] == "stop" {
		return nil
	}
	readOnly := map[string]bool{"echo": true, "pwd": true, "ls": true, "cat": true, "head": true, "tail": true, "df": true, "du": true, "ps": true, "whoami": true, "date": true, "uname": true, "journalctl": true}
	if readOnly[words[0]] {
		return nil
	}
	if words[0] == "docker" && len(words) >= 2 && map[string]bool{"ps": true, "logs": true, "inspect": true}[words[1]] {
		return nil
	}
	fileCommands := map[string]bool{"mv": true, "cp": true, "rm": true, "mkdir": true, "touch": true, "chmod": true}
	if !fileCommands[words[0]] {
		return fmt.Errorf("不允许执行 %q；仅支持主目录文件操作、只读命令和停止服务", words[0])
	}
	home := commandHomeDirectory(user)
	start := 1
	if words[0] == "chmod" {
		for start < len(words) && strings.HasPrefix(words[start], "-") {
			start++
		}
		if start < len(words) {
			start++
		}
	}
	pathCount := 0
	for _, word := range words[start:] {
		if strings.HasPrefix(word, "-") {
			continue
		}
		pathCount++
		if !commandPathInsideHome(word, home) {
			return fmt.Errorf("文件操作仅限 %s 目录内", home)
		}
	}
	if pathCount == 0 {
		return errors.New("文件操作缺少有效路径")
	}
	return nil
}

type limitedCommandOutput struct {
	mu        sync.Mutex
	data      []byte
	limit     int
	truncated bool
}

func (output *limitedCommandOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	remaining := output.limit - len(output.data)
	if remaining > 0 {
		if len(data) > remaining {
			output.data = append(output.data, data[:remaining]...)
			output.truncated = true
		} else {
			output.data = append(output.data, data...)
		}
	} else {
		output.truncated = true
	}
	return len(data), nil
}
func (output *limitedCommandOutput) String() string {
	output.mu.Lock()
	defer output.mu.Unlock()
	result := string(output.data)
	if output.truncated {
		result += "\n[输出过长，已截断]"
	}
	return result
}

func runCommandSSH(request commandExecutionRequest) (string, error) {
	client, err := newCommandSSHClient(commandConnectionRequest{Host: request.Host, Port: request.Port, User: request.User, Auth: request.Auth, Secret: request.Secret, Fingerprint: request.Fingerprint})
	if err != nil {
		return "", err
	}
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	output := &limitedCommandOutput{limit: 1 << 20}
	session.Stdout, session.Stderr = output, output
	done := make(chan error, 1)
	go func() { done <- session.Run(request.Command) }()
	select {
	case err := <-done:
		return output.String(), err
	case <-time.After(2 * time.Minute):
		_ = session.Close()
		return output.String(), errors.New("命令执行超时（限制 2 分钟）")
	}
}

func (s *server) handleCommandExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var request commandExecutionRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 128<<10)).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "执行参数无效"})
		return
	}
	request.Host, request.Port, request.User, request.Auth, request.Fingerprint, request.Command = strings.TrimSpace(request.Host), strings.TrimSpace(request.Port), strings.TrimSpace(request.User), strings.TrimSpace(request.Auth), strings.TrimSpace(request.Fingerprint), strings.TrimSpace(request.Command)
	if request.Port == "" {
		request.Port = "22"
	}
	if request.Host == "" || request.User == "" || request.Secret == "" || request.Fingerprint == "" || request.Command == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "服务器连接信息或指令不完整"})
		return
	}
	if err := validateRemoteCommand(request.Command, request.User); err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return
	}
	output, err := commandExecutionRun(request)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "命令执行失败：" + err.Error(), "output": output})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": output})
}

func newServer() *server {
	settings := loadAppSettings()
	maxStoredLogs = configuredLogCacheCapacity()
	s := &server{
		nodes:                 []Node{},
		logs:                  []LogEntry{},
		processed:             0,
		historyRange:          "30m",
		rules:                 map[string]bool{"mask": true, "structure": true, "noise": false},
		ruleOrder:             defaultRuleOrder(),
		subscribers:           make(map[chan LogEntry]struct{}),
		containerNames:        make(map[string]map[string]string),
		containerLogs:         make(map[string][]LogEntry),
		historyCoverage:       make(map[string]time.Time),
		historyLoads:          make(map[string]struct{}),
		historyLoadGeneration: make(map[string]uint64),
		streams:               make(map[string]struct{}),
		nodeContexts:          make(map[string]context.Context),
		nodeCancels:           make(map[string]context.CancelFunc),
		settings:              settings,
		store:                 newConfigStore(),
	}
	// Nodes and AI providers live in two JSON files, so a fresh install needs
	// no database and no configuration step. The resolved path is logged
	// because it depends on whether the executable's directory is writable.
	log.Printf("node storage: %s and %s", nodesFilePath(), modelsFilePath())
	if err := s.loadNodes(); err != nil {
		// A corrupt or unreadable file must not stop the service: report it and
		// start empty so the user can still reach the settings page to fix it.
		log.Printf("load node configuration failed: %v; starting with no nodes", err)
	}
	s.startPersistedNodes()
	return s
}

func configuredLogCacheCapacity() int {
	value := strings.TrimSpace(os.Getenv("LOG_AGENT_MAX_STORED_LOGS"))
	if value == "" {
		return defaultMaxStoredLogs
	}

	capacity, err := strconv.Atoi(value)
	if err != nil || capacity <= 0 {
		log.Printf("invalid LOG_AGENT_MAX_STORED_LOGS=%q; using %d", value, defaultMaxStoredLogs)
		return defaultMaxStoredLogs
	}
	if capacity > maxAllowedStoredLogs {
		log.Printf("LOG_AGENT_MAX_STORED_LOGS=%d exceeds maximum; using %d", capacity, maxAllowedStoredLogs)
		return maxAllowedStoredLogs
	}
	return capacity
}

func settingsFilePath() string {
	if path := strings.TrimSpace(os.Getenv("LOG_AGENT_SETTINGS_FILE")); path != "" {
		return path
	}
	return "log-agent-settings.json"
}

func defaultAppSettings() appSettings {
	environment := firstNonEmptyEnv("LOG_AGENT_ENVIRONMENT", "LOG_AGENT_ENV", "APP_ENV")
	if environment == "" {
		environment = "production"
	}
	return appSettings{
		Environment: environment,
		AdminToken:  os.Getenv("LOG_AGENT_ADMIN_TOKEN"),
	}
}

func firstNonEmptyEnv(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

func loadAppSettings() appSettings {
	settings := defaultAppSettings()
	content, err := os.ReadFile(settingsFilePath())
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("read settings file failed: %v; using environment settings", err)
		}
		return settings
	}
	var saved appSettings
	if err := json.Unmarshal(content, &saved); err != nil {
		log.Printf("parse settings file failed: %v; using environment settings", err)
		return settings
	}
	if strings.TrimSpace(saved.Environment) != "" {
		settings.Environment = strings.TrimSpace(saved.Environment)
	}
	if saved.AdminToken != "" {
		settings.AdminToken = saved.AdminToken
	} else {
		settings.AdminToken = ""
	}
	return settings
}

func saveAppSettings(settings appSettings) error {
	content, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return err
	}
	return writeFileAtomic(settingsFilePath(), append(content, '\n'))
}

func (s *server) loadNodes() error {
	return s.loadNodesFromStore()
}

// loadNodesFromStore reads nodes.json and hydrates the live node list, wiring
// up each node's context and cancel func exactly as the database path does.
func (s *server) loadNodesFromStore() error {
	if s.store == nil {
		return nil
	}
	records, err := s.store.listNodes()
	if err != nil {
		return err
	}
	loaded := make([]Node, 0, len(records))
	for _, record := range records {
		baseURL, containerID, parseErr := parseDozzleURL(record.Address)
		if parseErr != nil {
			log.Printf("skip node %q: %v", record.Name, parseErr)
			continue
		}
		loaded = append(loaded, Node{
			ID:          nodeIDForDB(record.ID),
			dbID:        record.ID,
			Name:        record.Name,
			URL:         record.Address,
			Style:       record.Style,
			Initial:     initialForName(record.Name),
			Status:      "connecting",
			baseURL:     baseURL,
			containerID: containerID,
		})
	}
	s.mu.Lock()
	s.nodes = loaded
	s.mu.Unlock()
	return nil
}

func (s *server) startPersistedNodes() {
	s.mu.RLock()
	nodes := append([]Node{}, s.nodes...)
	s.mu.RUnlock()
	for _, node := range nodes {
		if node.baseURL == "" {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		s.mu.Lock()
		s.nodeContexts[node.ID] = ctx
		s.nodeCancels[node.ID] = cancel
		s.mu.Unlock()
		go s.connectNode(ctx, node.ID)
	}
}

func nodeIDForDB(dbID int64) string {
	return fmt.Sprintf("node-db-%d", dbID)
}

func initialForName(name string) string {
	runes := []rune(strings.TrimSpace(name))
	if len(runes) == 0 {
		return "?"
	}
	return string(runes[0])
}

func normalizeNodeAddress(address string) string {
	address = strings.TrimSpace(address)
	if address != "" && !strings.Contains(address, "://") {
		return "http://" + address
	}
	return address
}

func normalizeNodeStyle(style string) string {
	style = strings.TrimSpace(style)
	if style == "" {
		return "HTTP / WebSocket"
	}
	return style
}

func (s *server) insertNodeRecord(name, address, style string) (int64, error) {
	if s.store == nil {
		return 0, nil
	}
	return s.store.insertNode(name, address, style)
}

func (s *server) updateNodeRecord(dbID int64, name, address, style string) error {
	if s.store == nil || dbID <= 0 {
		return nil
	}
	return s.store.updateNode(dbID, name, address, style)
}

func (s *server) deleteNodeRecord(dbID int64) error {
	if s.store == nil || dbID <= 0 {
		return nil
	}
	return s.store.deleteNode(dbID)
}

func (s *server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	logs := append([]LogEntry{}, s.logs...)
	if rawSince := strings.TrimSpace(r.URL.Query().Get("since")); rawSince != "" {
		if since, err := strconv.ParseInt(rawSince, 10, 64); err == nil && since > 0 {
			logs = logs[:0]
			for _, entry := range s.logs {
				if entry.ID > since {
					logs = append(logs, entry)
				}
			}
		}
	}
	storage := storageStats{
		Used: len(s.logs), Capacity: maxStoredLogs,
		Evicted: s.evictedLogs, LastEvictedAt: s.lastEvictedAt,
	}
	if storage.Capacity > 0 {
		storage.Percent = (storage.Used*100 + storage.Capacity - 1) / storage.Capacity
		if storage.Percent > 100 {
			storage.Percent = 100
		}
	}
	writeJSON(w, http.StatusOK, bootstrapResponse{
		Nodes: append([]Node{}, s.nodes...), Logs: logs, Processed: s.processed, Range: s.historyRange,
		HistoryLoading: s.historyPending > 0, Rules: copyRules(s.rules), RuleOrder: copyRuleOrder(s.ruleOrder), Storage: storage,
		StorageMode: string(backendForRequest(r)),
	})
}

func (s *server) handleClearLogCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	s.mu.Lock()
	cleared := len(s.logs)
	s.logs = nil
	s.containerLogs = make(map[string][]LogEntry)
	s.historyCoverage = make(map[string]time.Time)
	s.evictedLogs = 0
	s.lastEvictedAt = 0
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"cleared": cleared,
		"storage": storageStats{Used: 0, Capacity: maxStoredLogs},
	})
}

func (s *server) handleRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var request ruleUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "rule and enabled are required"})
		return
	}
	updates := request.Rules
	if request.Rule != "" {
		if request.Enabled == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "rule and enabled are required"})
			return
		}
		updates = map[string]bool{request.Rule: *request.Enabled}
	}
	if len(updates) == 0 && len(request.Order) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "rule update or order is required"})
		return
	}
	for rule := range updates {
		if !isKnownRule(rule) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown rule"})
			return
		}
	}
	if len(request.Order) > 0 && !isValidRuleOrder(request.Order) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "order must contain every rule exactly once"})
		return
	}

	s.mu.Lock()
	for rule, enabled := range updates {
		s.rules[rule] = enabled
	}
	if len(request.Order) > 0 {
		s.ruleOrder = copyRuleOrder(request.Order)
	}
	rules := copyRules(s.rules)
	order := copyRuleOrder(s.ruleOrder)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, struct {
		Rules map[string]bool `json:"rules"`
		Order []string        `json:"order"`
	}{Rules: rules, Order: order})
}

func isKnownRule(rule string) bool {
	switch rule {
	case "mask", "structure", "noise":
		return true
	default:
		return false
	}
}

func copyRules(rules map[string]bool) map[string]bool {
	copy := make(map[string]bool, len(rules))
	for name, enabled := range rules {
		copy[name] = enabled
	}
	return copy
}

func defaultRuleOrder() []string {
	return []string{"mask", "structure", "noise"}
}

func isValidRuleOrder(order []string) bool {
	if len(order) != len(defaultRuleOrder()) {
		return false
	}
	seen := make(map[string]struct{}, len(order))
	for _, rule := range order {
		if !isKnownRule(rule) {
			return false
		}
		if _, duplicate := seen[rule]; duplicate {
			return false
		}
		seen[rule] = struct{}{}
	}
	return true
}

func copyRuleOrder(order []string) []string {
	if !isValidRuleOrder(order) {
		return defaultRuleOrder()
	}
	return append([]string{}, order...)
}

func (s *server) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	nodeID := strings.TrimSpace(r.URL.Query().Get("node"))
	containerID := strings.TrimSpace(r.URL.Query().Get("container"))
	if nodeID == "" || containerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node and container are required"})
		return
	}

	rangeKey, ok := normalizeLogRange(r.URL.Query().Get("range"))
	if !ok {
		s.mu.RLock()
		rangeKey = s.historyRange
		s.mu.RUnlock()
	}
	key := containerLogKey(nodeID, containerID)
	s.mu.RLock()
	logs := append([]LogEntry{}, s.containerLogs[key]...)
	s.mu.RUnlock()

	loading := s.ensureContainerHistory(nodeID, containerID, rangeKey)
	writeJSON(w, http.StatusOK, containerLogsResponse{Logs: logs, Loading: loading})
}

// handleContainerHistorySearch searches the entire requested time range at
// Dozzle. It deliberately does not reuse the capped in-memory browse cache:
// a busy container can have far more than 5,000 records in a week, and a
// search must not silently omit matches that happen to be on older pages.
func (s *server) handleContainerHistorySearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	nodeID := strings.TrimSpace(r.URL.Query().Get("node"))
	containerID := strings.TrimSpace(r.URL.Query().Get("container"))
	needle := strings.ToLower(strings.Join(strings.Fields(r.URL.Query().Get("q")), " "))
	if nodeID == "" || containerID == "" || needle == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node, container and q are required"})
		return
	}
	if len([]rune(needle)) > 512 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q is too long"})
		return
	}
	rangeKey, ok := normalizeLogRange(r.URL.Query().Get("range"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "range must be one of 30m, 5h, 1d, 1w"})
		return
	}
	s.mu.RLock()
	node, found := s.nodeByIDLocked(nodeID)
	s.mu.RUnlock()
	if !found || node.hostID == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "container node is unavailable"})
		return
	}
	duration, _ := logRangeDuration(rangeKey)
	now := time.Now().UTC()
	ctx, cancel := context.WithTimeout(r.Context(), historyFetchTimeout)
	defer cancel()
	logs, err := s.searchDozzleHistory(ctx, node, node.hostID, containerID, now.Add(-duration), now, needle)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		writeJSON(w, status, map[string]string{"error": "筛选完整时间范围日志失败: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, containerHistorySearchResponse{Logs: logs})
}

func (s *server) handleContainerHistoryPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	nodeID, containerID := strings.TrimSpace(r.URL.Query().Get("node")), strings.TrimSpace(r.URL.Query().Get("container"))
	before, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("before")), 10, 64)
	if nodeID == "" || containerID == "" || err != nil || before <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node, container and before are required"})
		return
	}
	rangeKey, ok := normalizeLogRange(r.URL.Query().Get("range"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid range"})
		return
	}
	s.mu.RLock()
	node, found := s.nodeByIDLocked(nodeID)
	s.mu.RUnlock()
	if !found || node.hostID == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "container node is unavailable"})
		return
	}
	duration, _ := logRangeDuration(rangeKey)
	from := time.Now().UTC().Add(-duration)
	to := time.UnixMilli(before).UTC().Add(-time.Millisecond)
	if !to.After(from) {
		writeJSON(w, http.StatusOK, containerHistoryPageResponse{Logs: []LogEntry{}, HasMore: false})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), historyFetchTimeout)
	defer cancel()
	oldest, count, logs, err := s.fetchDozzleHistorySearchPage(ctx, node, node.hostID, containerID, from, to, "")
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "加载更早日志失败: " + err.Error()})
		return
	}
	hasMore := count == dozzleHistoryPageSize && !oldest.IsZero() && oldest.After(from)
	next := int64(0)
	if hasMore {
		next = oldest.UnixMilli()
	}
	writeJSON(w, http.StatusOK, containerHistoryPageResponse{Logs: logs, HasMore: hasMore, NextBefore: next})
}

func (s *server) ensureContainerHistory(nodeID, containerID, rangeKey string) bool {
	duration, ok := logRangeDuration(rangeKey)
	if !ok {
		return false
	}
	desiredFrom := time.Now().UTC().Add(-duration)
	key := containerLogKey(nodeID, containerID)
	s.mu.RLock()
	node, nodeOK := s.nodeByIDLocked(nodeID)
	ctx := s.nodeContexts[nodeID]
	generation := s.historyGeneration
	_, alreadyLoading := s.historyLoads[key]
	coverage, covered := s.historyCoverage[key]
	s.mu.RUnlock()
	if !nodeOK || node.hostID == "" || ctx == nil {
		return false
	}
	if covered && !coverage.After(desiredFrom) {
		return false
	}
	if !alreadyLoading {
		s.launchHistoryFetchWindow(ctx, node, node.hostID, containerID, desiredFrom, time.Now().UTC(), generation, true)
	}
	s.mu.RLock()
	_, loading := s.historyLoads[key]
	s.mu.RUnlock()
	return loading
}

func containerLogKey(nodeID, containerID string) string {
	return nodeID + "::" + containerID
}

func (s *server) handleLogRange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	rangeKey, ok := normalizeLogRange(r.URL.Query().Get("range"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "range must be one of 30m, 5h, 1d, 1w"})
		return
	}

	duration, _ := logRangeDuration(rangeKey)
	now := time.Now().UTC()
	requestedNodeIDs := make(map[string]struct{})
	for _, nodeID := range r.URL.Query()["node"] {
		if nodeID = strings.TrimSpace(nodeID); nodeID != "" {
			requestedNodeIDs[nodeID] = struct{}{}
		}
	}
	requestedContainers := make(map[string]map[string]struct{})
	for _, scope := range r.URL.Query()["container"] {
		parts := strings.SplitN(strings.TrimSpace(scope), "::", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			continue
		}
		if requestedContainers[parts[0]] == nil {
			requestedContainers[parts[0]] = make(map[string]struct{})
		}
		requestedContainers[parts[0]][parts[1]] = struct{}{}
	}

	s.mu.Lock()
	nodes := append([]Node{}, s.nodes...)
	scopedNodeIDs := make(map[string]struct{})
	for _, node := range nodes {
		if _, requested := requestedNodeIDs[node.ID]; requested {
			scopedNodeIDs[node.ID] = struct{}{}
		}
	}
	for nodeID := range requestedContainers {
		for _, node := range nodes {
			if node.ID == nodeID {
				scopedNodeIDs[nodeID] = struct{}{}
				break
			}
		}
	}
	scopeIDs := make([]string, 0, len(scopedNodeIDs))
	for nodeID := range scopedNodeIDs {
		scopeIDs = append(scopeIDs, nodeID)
	}
	sort.Strings(scopeIDs)
	containerScopeIDs := make([]string, 0)
	for nodeID, containerIDs := range requestedContainers {
		if _, selected := scopedNodeIDs[nodeID]; !selected {
			continue
		}
		for containerID := range containerIDs {
			containerScopeIDs = append(containerScopeIDs, containerLogKey(nodeID, containerID))
		}
	}
	sort.Strings(containerScopeIDs)
	scopeKey := "nodes=" + strings.Join(scopeIDs, ",") + "|containers=" + strings.Join(containerScopeIDs, ",")
	scopeChanged := s.historyNodeScope != scopeKey
	if s.historyRange == rangeKey && !scopeChanged {
		s.mu.Unlock()
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "unchanged", "range": rangeKey})
		return
	}
	previousDuration, previousDurationOK := logRangeDuration(s.historyRange)
	// Expanding the window can safely reuse the cache and fetch only the
	// missing older segment. Narrowing normally only changes the view, unless
	// the wider request is still loading: that partial cache is discarded and
	// the smaller window starts from a clean, exact snapshot.
	strictReload := previousDurationOK && duration < previousDuration && s.historyPending > 0
	loadOlderLogs := scopeChanged || !previousDurationOK || duration > previousDuration || strictReload
	s.historyRange = rangeKey
	s.historyNodeScope = scopeKey
	s.historyGeneration++
	generation := s.historyGeneration
	s.historyPending = 0
	if strictReload {
		s.logs = []LogEntry{}
		s.containerLogs = make(map[string][]LogEntry)
		s.historyCoverage = make(map[string]time.Time)
		// Stale fetches carry the previous generation and cannot add data to the
		// new cache. Track load ownership so their deferred cleanup also cannot
		// remove a fresh load for the same container.
		s.historyLoads = make(map[string]struct{})
		s.historyLoadGeneration = make(map[string]uint64)
	}
	targets := make([]historyTarget, 0)
	if loadOlderLogs {
		from := now.Add(-duration)
		for _, node := range nodes {
			if len(scopedNodeIDs) > 0 {
				if _, selected := scopedNodeIDs[node.ID]; !selected {
					continue
				}
			}
			if node.hostID == "" {
				continue
			}
			if selectedContainerIDs := requestedContainers[node.ID]; len(selectedContainerIDs) > 0 {
				for containerID := range selectedContainerIDs {
					s.addHistoryTargetLocked(&targets, node, containerID, from, now, true)
				}
				continue
			}
			containerIDs := s.containerNames[node.ID]
			if len(containerIDs) > 0 {
				for containerID := range containerIDs {
					s.addHistoryTargetLocked(&targets, node, containerID, from, now, false)
				}
				continue
			}
			if node.containerID != "" {
				s.addHistoryTargetLocked(&targets, node, node.containerID, from, now, false)
				continue
			}
			for _, container := range node.Containers {
				if container.ID != "" {
					s.addHistoryTargetLocked(&targets, node, container.ID, from, now, false)
				}
			}
		}
	}
	s.mu.Unlock()

	for _, target := range targets {
		s.launchHistoryFetchWindow(target.ctx, target.node, target.node.hostID, target.containerID, target.from, target.to, generation, target.allowWhenSharedCacheFull)
	}
	status := "filtered"
	if strictReload {
		status = "reloading"
	} else if loadOlderLogs && len(targets) > 0 {
		status = "loading"
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": status, "range": rangeKey})
}

type historyTarget struct {
	node                     Node
	containerID              string
	ctx                      context.Context
	from                     time.Time
	to                       time.Time
	allowWhenSharedCacheFull bool
}

func (s *server) addHistoryTargetLocked(targets *[]historyTarget, node Node, containerID string, from, to time.Time, allowWhenSharedCacheFull bool) {
	key := containerLogKey(node.ID, containerID)
	boundary, hasBoundary := s.historyCoverage[key]
	if earliest, ok := s.earliestContainerLogLocked(node.ID, containerID); ok && (!hasBoundary || earliest.Before(boundary)) {
		boundary = earliest
		hasBoundary = true
	}
	if hasBoundary {
		if !boundary.After(from) {
			return
		}
		// Include the boundary log. The ingestion path de-duplicates it when
		// the remote endpoint treats the upper bound as inclusive.
		to = boundary.Add(time.Millisecond)
	}
	*targets = append(*targets, historyTarget{
		node: node, containerID: containerID, ctx: s.nodeContexts[node.ID], from: from, to: to, allowWhenSharedCacheFull: allowWhenSharedCacheFull,
	})
}

func (s *server) markHistoryCoverage(nodeID, containerID string, from time.Time, generation uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.historyGeneration {
		return
	}
	if s.historyCoverage == nil {
		s.historyCoverage = make(map[string]time.Time)
	}
	key := containerLogKey(nodeID, containerID)
	if previous, ok := s.historyCoverage[key]; !ok || from.Before(previous) {
		s.historyCoverage[key] = from
	}
}

func (s *server) earliestContainerLogLocked(nodeID, containerID string) (time.Time, bool) {
	logs := s.containerLogs[containerLogKey(nodeID, containerID)]
	if len(logs) == 0 {
		return time.Time{}, false
	}
	earliest := logs[0].Timestamp
	for _, entry := range logs[1:] {
		if entry.Timestamp > 0 && (earliest <= 0 || entry.Timestamp < earliest) {
			earliest = entry.Timestamp
		}
	}
	if earliest <= 0 {
		return time.Time{}, false
	}
	return time.UnixMilli(earliest).UTC(), true
}

func (s *server) launchHistoryFetch(parent context.Context, node Node, hostID, containerID, rangeKey string, generation uint64) {
	duration, ok := logRangeDuration(rangeKey)
	if !ok {
		return
	}
	now := time.Now().UTC()
	s.launchHistoryFetchWindow(parent, node, hostID, containerID, now.Add(-duration), now, generation, false)
}

func (s *server) launchHistoryFetchWindow(parent context.Context, node Node, hostID, containerID string, from, to time.Time, generation uint64, allowWhenSharedCacheFull bool) {
	if !from.Before(to) {
		return
	}
	s.mu.Lock()
	if generation != s.historyGeneration {
		s.mu.Unlock()
		return
	}
	key := containerLogKey(node.ID, containerID)
	if s.historyLoads == nil {
		s.historyLoads = make(map[string]struct{})
	}
	if s.historyLoadGeneration == nil {
		s.historyLoadGeneration = make(map[string]uint64)
	}
	if _, exists := s.historyLoads[key]; exists {
		s.mu.Unlock()
		return
	}
	s.historyLoads[key] = struct{}{}
	s.historyLoadGeneration[key] = generation
	s.historyPending++
	s.mu.Unlock()

	go func() {
		defer s.completeHistoryFetch(generation)
		defer func() {
			s.mu.Lock()
			if s.historyLoadGeneration[key] == generation {
				delete(s.historyLoads, key)
				delete(s.historyLoadGeneration, key)
			}
			s.mu.Unlock()
		}()
		s.fetchDozzleHistoryWithCachePolicy(parent, node, hostID, containerID, from, to, generation, allowWhenSharedCacheFull)
	}()
}

func (s *server) completeHistoryFetch(generation uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.historyGeneration || s.historyPending == 0 {
		return
	}
	s.historyPending--
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "healthy", "service": "dozzle-ops"})
}

type settingsUpdateRequest struct {
	Environment string `json:"environment"`
	AdminToken  string `json:"adminToken"`
}

func (s *server) currentAdminToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.settings.AdminToken)
}

func authorizedAdminRequest(r *http.Request, token string) bool {
	provided := []byte(r.Header.Get("X-Log-Agent-Admin-Token"))
	expected := []byte(strings.TrimSpace(token))
	return len(expected) > 0 && len(provided) == len(expected) && subtle.ConstantTimeCompare(provided, expected) == 1
}

func settingsView(settings appSettings) settingsResponse {
	return settingsResponse{
		Environment:          settings.Environment,
		AdminTokenConfigured: strings.TrimSpace(settings.AdminToken) != "",
	}
}

func (s *server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.mu.RLock()
		settings := s.settings
		s.mu.RUnlock()
		writeJSON(w, http.StatusOK, settingsView(settings))
		return
	}
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	currentToken := s.currentAdminToken()
	if currentToken != "" && !authorizedAdminRequest(r, currentToken) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "admin token required"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var request settingsUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid settings"})
		return
	}

	s.mu.RLock()
	previous := s.settings
	s.mu.RUnlock()
	settings := previous
	settings.Environment = strings.TrimSpace(request.Environment)
	if settings.Environment == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment is required"})
		return
	}
	if len([]rune(settings.Environment)) > 50 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment is too long"})
		return
	}
	if strings.TrimSpace(request.AdminToken) != "" {
		settings.AdminToken = strings.TrimSpace(request.AdminToken)
	}

	if err := saveAppSettings(settings); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("save settings failed: %v", err)})
		return
	}

	s.mu.Lock()
	s.settings = settings
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, settingsView(settings))
}

func (s *server) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if token := s.currentAdminToken(); token != "" && !authorizedAdminRequest(r, token) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "admin token required"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var request struct {
		AdminToken string `json:"adminToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid admin settings"})
		return
	}
	s.mu.RLock()
	settings := s.settings
	s.mu.RUnlock()
	if token := strings.TrimSpace(request.AdminToken); token != "" {
		settings.AdminToken = token
	}
	if err := saveAppSettings(settings); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("save admin settings failed: %v", err)})
		return
	}
	s.mu.Lock()
	s.settings = settings
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, settingsView(settings))
}

func (s *server) handleEnvironmentSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if token := s.currentAdminToken(); token != "" && !authorizedAdminRequest(r, token) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "admin token required"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	var request struct {
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || strings.TrimSpace(request.Environment) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment is required"})
		return
	}
	s.mu.RLock()
	settings := s.settings
	s.mu.RUnlock()
	settings.Environment = strings.TrimSpace(request.Environment)
	if err := saveAppSettings(settings); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("save environment settings failed: %v", err)})
		return
	}
	s.mu.Lock()
	s.settings = settings
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, settingsView(settings))
}

func (s *server) handleAIStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	profile := configuredAIProfile()
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": profile.BaseURL != "",
		"model":      profile.Model,
	})
}

func (s *server) handleAIProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		profiles, err := s.loadAIProfileViews()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load AI profiles failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"profiles": profiles})
		return
	}
	if s.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "AI profile storage is not available"})
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !authorizedAdminRequest(r, s.currentAdminToken()) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "admin token required"})
		return
	}
	// Provider records carry API keys; a remote caller must keep its own in its
	// browser rather than uploading them to this host.
	if !requireFileBackend(w, r) {
		return
	}
	if r.Method == http.MethodDelete {
		var id int64
		if _, err := fmt.Sscanf(r.URL.Query().Get("id"), "%d", &id); err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid profile id"})
			return
		}
		// upsertModel keeps the stored key when the request omits one, so
		// editing a provider does not wipe its credentials.
		if err := s.store.deleteModel(id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete AI profile failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var request struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		BaseURL string `json:"baseURL"`
		APIKey  string `json:"apiKey"`
		Type    string `json:"type"`
		Models  []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid AI profile"})
		return
	}
	request.Name, request.BaseURL = strings.TrimSpace(request.Name), strings.TrimRight(strings.TrimSpace(request.BaseURL), "/")
	modelNames := make([]string, 0, len(request.Models))
	for _, model := range request.Models {
		if name := strings.TrimSpace(model.Name); name != "" {
			modelNames = append(modelNames, name)
		}
	}
	profileType := int64(2)
	if strings.EqualFold(request.Type, "anthropic") {
		profileType = 1
	}
	if request.Name == "" || request.BaseURL == "" || len(modelNames) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, base URL and model are required"})
		return
	}
	var id int64
	_, scanErr := fmt.Sscanf(request.ID, "ai-profile-db-%d", &id)
	if scanErr != nil || id <= 0 {
		id = 0
	}
	// upsertModel keeps the stored key when the request omits one, so editing
	// a provider does not wipe its credentials.
	id, err := s.store.upsertModel(id, request.Name, request.BaseURL, request.APIKey, int(profileType), modelNames)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save AI profile failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": fmt.Sprintf("ai-profile-db-%d", id)})
}

type aiModelView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type aiProfileView struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	BaseURL string        `json:"baseURL"`
	Type    string        `json:"type"`
	Enabled bool          `json:"enabled"`
	Models  []aiModelView `json:"models"`
}

// profileViewsFromStored converts file-backed records into the same shape the
// database path returns, so the model picker does not care which backend is
// active.
func profileViewsFromStored(records []storedModel) []aiProfileView {
	profiles := make([]aiProfileView, 0, len(records))
	for _, record := range records {
		profile := aiProfileView{
			ID:      fmt.Sprintf("ai-profile-db-%d", record.ID),
			Name:    strings.TrimSpace(record.Name),
			BaseURL: strings.TrimRight(strings.TrimSpace(record.BaseURL), "/"),
			Type:    "openai",
			Enabled: true,
		}
		if record.Type == 1 {
			profile.Type = "anthropic"
		}
		for index, modelName := range strings.Split(record.ModelName, ",") {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				continue
			}
			profile.Models = append(profile.Models, aiModelView{
				ID:   fmt.Sprintf("db-model-%d-%d", record.ID, index),
				Name: modelName,
			})
		}
		if profile.Name != "" && profile.BaseURL != "" && len(profile.Models) > 0 {
			profiles = append(profiles, profile)
		}
	}
	return profiles
}

// loadAIProfileViews returns the configured providers from the JSON store.
func (s *server) loadAIProfileViews() ([]aiProfileView, error) {
	if s.store == nil {
		return []aiProfileView{}, nil
	}
	records, err := s.store.listModels()
	if err != nil {
		return nil, err
	}
	return profileViewsFromStored(records), nil
}

func (s *server) loadAIProfileSecret(id int64, modelName string) (aiProfile, error) {
	if id <= 0 {
		return aiProfile{}, errors.New("AI profile is not available")
	}
	var profile aiProfile
	var profileType int
	var modelNames string

	if s.store == nil {
		return aiProfile{}, errors.New("AI profile is not available")
	}
	record, err := s.store.findModel(id)
	if err != nil {
		return aiProfile{}, err
	}
	profile.Name = record.Name
	profile.BaseURL = record.BaseURL
	profile.APIKey = record.APIKey
	profileType = record.Type
	modelNames = record.ModelName

	profile.Type = "openai"
	if profileType == 1 {
		profile.Type = "anthropic"
	}
	profile.Model = strings.TrimSpace(modelName)
	if profile.Model == "" {
		for _, name := range strings.Split(modelNames, ",") {
			if name = strings.TrimSpace(name); name != "" {
				profile.Model = name
				break
			}
		}
	}
	return normalizeAIProfile(profile), nil
}

func (s *server) handleAIChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	var request aiChatRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid AI chat request"})
		return
	}
	if len(request.Messages) == 0 || len(request.Messages) > 20 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "AI chat messages must contain between 1 and 20 items"})
		return
	}
	if len(request.Logs) > 20 {
		request.Logs = request.Logs[:20]
	}
	attachments, err := normalizeAIAttachments(request.Attachments)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	profile := configuredAIProfile()
	if request.Config != nil {
		if request.Config.ProfileID > 0 {
			loadedProfile, err := s.loadAIProfileSecret(request.Config.ProfileID, request.Config.Model)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "load AI profile failed"})
				return
			}
			profile = loadedProfile
		} else {
			profile = normalizeAIProfile(*request.Config)
		}
	}
	if profile.BaseURL == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "AI provider is not configured"})
		return
	}
	if profile.Model == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "AI model is required"})
		return
	}

	messages := []aiMessage{{
		Role:    "system",
		Content: "You are a log analysis assistant. Analyze the supplied logs carefully. Do not invent facts. If the logs are insufficient, say what is missing. Answer in Chinese unless the user asks otherwise.",
	}}
	if contextMessage := formatAIContext(request.Logs); contextMessage != "" {
		messages = append(messages, aiMessage{Role: "user", Content: contextMessage})
	}
	for _, message := range request.Messages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		if len([]rune(content)) > 8000 {
			content = string([]rune(content)[:8000])
		}
		messages = append(messages, aiMessage{Role: role, Content: content})
	}
	if len(messages) == 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "AI chat message is empty"})
		return
	}

	var body []byte
	if profile.Type == "anthropic" {
		// Anthropic Messages API expects the system instruction at the top level,
		// not as a message with role=system.
		systemPrompt := ""
		anthropicMessages := messages
		if len(messages) > 0 && messages[0].Role == "system" {
			systemPrompt = messages[0].Content
			anthropicMessages = messages[1:]
		}
		payload := struct {
			Model     string              `json:"model"`
			System    string              `json:"system,omitempty"`
			Messages  []aiProviderMessage `json:"messages"`
			MaxTokens int                 `json:"max_tokens"`
		}{Model: profile.Model, System: systemPrompt, Messages: anthropicProviderMessages(anthropicMessages, attachments), MaxTokens: 4096}
		body, err = json.Marshal(payload)
	} else {
		payload := struct {
			Model    string              `json:"model"`
			Messages []aiProviderMessage `json:"messages"`
		}{Model: profile.Model, Messages: openAIProviderMessages(messages, attachments)}
		body, err = json.Marshal(payload)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode AI request failed"})
		return
	}

	endpoint := strings.TrimRight(profile.BaseURL, "/") + "/chat/completions"
	if profile.Type == "anthropic" {
		baseURL := strings.TrimRight(profile.BaseURL, "/")
		if strings.HasSuffix(baseURL, "/messages") {
			endpoint = baseURL
		} else if strings.HasSuffix(baseURL, "/v1") {
			endpoint = baseURL + "/messages"
		} else {
			endpoint = baseURL + "/v1/messages"
		}
	}
	providerRequest, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("invalid AI provider URL: %v", err)})
		return
	}
	providerRequest.Header.Set("Content-Type", "application/json")
	providerRequest.Header.Set("Accept", "application/json")
	if profile.Type == "anthropic" {
		providerRequest.Header.Set("x-api-key", profile.APIKey)
		providerRequest.Header.Set("Authorization", "Bearer "+profile.APIKey)
		providerRequest.Header.Set("anthropic-version", "2023-06-01")
	} else if profile.APIKey != "" {
		providerRequest.Header.Set("Authorization", "Bearer "+profile.APIKey)
	}
	providerResponse, err := (&http.Client{Timeout: 180 * time.Second}).Do(providerRequest)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("AI provider request failed: %v", err)})
		return
	}
	defer providerResponse.Body.Close()
	providerBody, err := io.ReadAll(io.LimitReader(providerResponse.Body, 2<<20))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "read AI provider response failed"})
		return
	}
	if providerResponse.StatusCode < http.StatusOK || providerResponse.StatusCode >= http.StatusMultipleChoices {
		detail := strings.TrimSpace(string(providerBody))
		if len([]rune(detail)) > 4000 {
			detail = string([]rune(detail)[:4000]) + "…"
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("AI provider returned HTTP %d: %s", providerResponse.StatusCode, detail)})
		return
	}
	var answer string
	if profile.Type == "anthropic" {
		var response struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(providerBody, &response); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid Anthropic response"})
			return
		}
		for _, item := range response.Content {
			if item.Type == "text" {
				answer += item.Text
			}
		}
	} else {
		var response aiChatResponse
		if err := json.Unmarshal(providerBody, &response); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid AI provider response"})
			return
		}
		if len(response.Choices) > 0 {
			answer = response.Choices[0].Message.Content
		}
	}
	if strings.TrimSpace(answer) == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "AI provider returned no answer"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"message": strings.TrimSpace(answer),
		"model":   profile.Model,
	})
}

func configuredAIProfile() aiProfile {
	return normalizeAIProfile(aiProfile{
		BaseURL: os.Getenv("LOG_AGENT_AI_BASE_URL"),
		APIKey:  os.Getenv("LOG_AGENT_AI_API_KEY"),
		Model:   os.Getenv("LOG_AGENT_AI_MODEL"),
		Type:    "openai",
	})
}

func normalizeAIProfile(profile aiProfile) aiProfile {
	profile.Name = strings.TrimSpace(profile.Name)
	profile.BaseURL = strings.TrimRight(strings.TrimSpace(profile.BaseURL), "/")
	profile.APIKey = strings.TrimSpace(profile.APIKey)
	profile.Model = strings.TrimSpace(profile.Model)
	profile.Type = strings.ToLower(strings.TrimSpace(profile.Type))
	if profile.Type != "anthropic" {
		profile.Type = "openai"
	}
	if profile.Model == "" {
		profile.Model = "gpt-4o-mini"
	}
	return profile
}

func formatAIContext(logs []LogEntry) string {
	if len(logs) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("以下是用户选中的日志上下文：\n")
	for index, entry := range logs {
		message := entry.Message
		if len([]rune(message)) > 4000 {
			message = string([]rune(message)[:4000]) + "…"
		}
		fmt.Fprintf(&builder, "\n[%d] time=%s level=%s node=%s container=%s\n%s\n", index+1, entry.Time, entry.Level, entry.Node, entry.Container, message)
	}
	return builder.String()
}

const (
	maxAIAttachmentCount = 5
	maxAIAttachmentBytes = 2 * 1024 * 1024
)

func isAIImageAttachment(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func isAITextAttachment(contentType string) bool {
	return strings.HasPrefix(contentType, "text/") || contentType == "application/json" || contentType == "application/xml" || contentType == "application/x-yaml" || contentType == "application/x-yml"
}

func normalizeAIAttachments(attachments []aiAttachment) ([]aiAttachment, error) {
	if len(attachments) > maxAIAttachmentCount {
		return nil, fmt.Errorf("at most %d attachments are allowed", maxAIAttachmentCount)
	}
	normalized := make([]aiAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		attachment.Name = strings.TrimSpace(attachment.Name)
		if attachment.Name == "" {
			attachment.Name = "attachment"
		}
		attachment.Type = strings.ToLower(strings.TrimSpace(strings.Split(attachment.Type, ";")[0]))
		if !isAIImageAttachment(attachment.Type) && !isAITextAttachment(attachment.Type) {
			return nil, fmt.Errorf("unsupported attachment type: %s", attachment.Name)
		}
		prefix := "data:" + attachment.Type + ";base64,"
		if !strings.HasPrefix(attachment.Data, prefix) {
			return nil, fmt.Errorf("invalid attachment data: %s", attachment.Name)
		}
		data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(attachment.Data, prefix))
		if err != nil || len(data) == 0 || len(data) > maxAIAttachmentBytes {
			return nil, fmt.Errorf("invalid or oversized attachment: %s", attachment.Name)
		}
		attachment.Data = base64.StdEncoding.EncodeToString(data)
		normalized = append(normalized, attachment)
	}
	return normalized, nil
}

func aiTextAttachmentContext(attachments []aiAttachment) string {
	var builder strings.Builder
	for _, attachment := range attachments {
		if !isAITextAttachment(attachment.Type) {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(attachment.Data)
		if err != nil {
			continue
		}
		text := string(data)
		if len([]rune(text)) > 12000 {
			text = string([]rune(text)[:12000]) + "…"
		}
		fmt.Fprintf(&builder, "\n\n附件文件：%s\n%s", attachment.Name, text)
	}
	return builder.String()
}

func openAIProviderMessages(messages []aiMessage, attachments []aiAttachment) []aiProviderMessage {
	providerMessages := make([]aiProviderMessage, 0, len(messages))
	for _, message := range messages {
		providerMessages = append(providerMessages, aiProviderMessage{Role: message.Role, Content: message.Content})
	}
	if len(attachments) == 0 {
		return providerMessages
	}
	target := -1
	for index := len(providerMessages) - 1; index >= 0; index-- {
		if providerMessages[index].Role == "user" {
			target = index
			break
		}
	}
	if target < 0 {
		providerMessages = append(providerMessages, aiProviderMessage{Role: "user"})
		target = len(providerMessages) - 1
	}
	text, _ := providerMessages[target].Content.(string)
	parts := []map[string]any{{"type": "text", "text": text + aiTextAttachmentContext(attachments)}}
	for _, attachment := range attachments {
		if isAIImageAttachment(attachment.Type) {
			parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:" + attachment.Type + ";base64," + attachment.Data}})
		}
	}
	providerMessages[target].Content = parts
	return providerMessages
}

func anthropicProviderMessages(messages []aiMessage, attachments []aiAttachment) []aiProviderMessage {
	providerMessages := make([]aiProviderMessage, 0, len(messages))
	for _, message := range messages {
		providerMessages = append(providerMessages, aiProviderMessage{Role: message.Role, Content: message.Content})
	}
	if len(attachments) == 0 {
		return providerMessages
	}
	target := -1
	for index := len(providerMessages) - 1; index >= 0; index-- {
		if providerMessages[index].Role == "user" {
			target = index
			break
		}
	}
	if target < 0 {
		providerMessages = append(providerMessages, aiProviderMessage{Role: "user"})
		target = len(providerMessages) - 1
	}
	text, _ := providerMessages[target].Content.(string)
	parts := []map[string]any{{"type": "text", "text": text + aiTextAttachmentContext(attachments)}}
	for _, attachment := range attachments {
		if isAIImageAttachment(attachment.Type) {
			parts = append(parts, map[string]any{"type": "image", "source": map[string]string{"type": "base64", "media_type": attachment.Type, "data": attachment.Data}})
		}
	}
	providerMessages[target].Content = parts
	return providerMessages
}

// requireFileBackend rejects a configuration write that did not originate on
// the machine running the service.
//
// A caller reaching us from elsewhere uses its own browser storage, so it has
// no business creating nodes on this server. Without this check, any visitor
// could append entries to the owner's nodes.json.
func requireFileBackend(w http.ResponseWriter, r *http.Request) bool {
	if backendForRequest(r) == backendFile {
		return true
	}
	writeJSON(w, http.StatusForbidden, map[string]string{
		"error": "该页面未在本机打开，节点配置保存在浏览器本地",
	})
	return false
}

func (s *server) handleNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		s.unbindNode(w, r.URL.Query().Get("id"))
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !requireFileBackend(w, r) {
		return
	}
	var request nodeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || strings.TrimSpace(request.Name) == "" || strings.TrimSpace(request.URL) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and url are required"})
		return
	}
	name := strings.TrimSpace(request.Name)
	rawURL := strings.TrimSpace(request.URL)
	style := normalizeNodeStyle(request.Style)
	baseURL, containerID, err := parseDozzleURL(rawURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	node := Node{
		ID:          fmt.Sprintf("node-%d", time.Now().UnixNano()),
		Name:        name,
		URL:         rawURL,
		Style:       style,
		Initial:     string([]rune(name)[0]),
		Status:      "connecting",
		baseURL:     baseURL,
		containerID: containerID,
	}
	s.mu.Lock()
	for _, existing := range s.nodes {
		if canonicalNodeURL(existing.URL) == canonicalNodeURL(rawURL) {
			s.mu.Unlock()
			writeJSON(w, http.StatusConflict, map[string]any{"error": "node already exists", "node": existing})
			return
		}
	}
	if dbID, err := s.insertNodeRecord(name, rawURL, style); err != nil {
		s.mu.Unlock()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("save node failed: %v", err)})
		return
	} else if dbID > 0 {
		node.ID = nodeIDForDB(dbID)
		node.dbID = dbID
	}
	s.nodes = append(s.nodes, node)
	ctx, cancel := context.WithCancel(context.Background())
	s.nodeContexts[node.ID] = ctx
	s.nodeCancels[node.ID] = cancel
	s.mu.Unlock()
	go s.connectNode(ctx, node.ID)
	writeJSON(w, http.StatusCreated, node)
}

func (s *server) handleNode(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/nodes/"), "/")
	if r.Method == http.MethodPut {
		s.updateNode(w, r, id)
		return
	}
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	// Deleting from another machine would remove a node the owner configured.
	if !requireFileBackend(w, r) {
		return
	}
	s.unbindNode(w, id)
}

func (s *server) updateNode(w http.ResponseWriter, r *http.Request, id string) {
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node id is required"})
		return
	}
	if !requireFileBackend(w, r) {
		return
	}
	var request nodeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || strings.TrimSpace(request.Name) == "" || strings.TrimSpace(request.URL) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and url are required"})
		return
	}
	name := strings.TrimSpace(request.Name)
	rawURL := strings.TrimSpace(request.URL)
	style := normalizeNodeStyle(request.Style)
	baseURL, containerID, err := parseDozzleURL(rawURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	s.mu.Lock()
	index := -1
	for i, node := range s.nodes {
		if node.ID == id {
			index = i
			continue
		}
		if canonicalNodeURL(node.URL) == canonicalNodeURL(rawURL) {
			s.mu.Unlock()
			writeJSON(w, http.StatusConflict, map[string]any{"error": "node already exists", "node": node})
			return
		}
	}
	if index < 0 {
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "node not found"})
		return
	}
	cancel := s.nodeCancels[id]
	updated := s.nodes[index]
	updated.Name = name
	updated.URL = rawURL
	updated.Style = style
	updated.Initial = initialForName(name)
	updated.Status = "connecting"
	updated.Error = ""
	updated.Warning = false
	updated.Latency = 0
	updated.Version = ""
	updated.Count = 0
	updated.Containers = nil
	updated.baseURL = baseURL
	updated.containerID = containerID
	if err := s.updateNodeRecord(updated.dbID, updated.Name, updated.URL, updated.Style); err != nil {
		s.mu.Unlock()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("update node failed: %v", err)})
		return
	}
	// The node context replaces the previous one only once the persisted
	// update succeeded, so the error path above cannot leak a cancel func.
	ctx, newCancel := context.WithCancel(context.Background())
	s.nodes[index] = updated
	s.nodeContexts[id] = ctx
	s.nodeCancels[id] = newCancel
	delete(s.containerNames, id)
	for key := range s.historyCoverage {
		if strings.HasPrefix(key, id+"::") {
			delete(s.historyCoverage, key)
		}
	}
	for key := range s.containerLogs {
		if strings.HasPrefix(key, id+"::") {
			delete(s.containerLogs, key)
		}
	}
	for key := range s.streams {
		if strings.HasPrefix(key, id+":") {
			delete(s.streams, key)
		}
	}
	filteredLogs := s.logs[:0]
	for _, entry := range s.logs {
		if entry.nodeID != id {
			filteredLogs = append(filteredLogs, entry)
		}
	}
	s.logs = filteredLogs
	s.historyGeneration++
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	go s.connectNode(ctx, id)
	writeJSON(w, http.StatusOK, updated)
}

func (s *server) unbindNode(w http.ResponseWriter, rawID string) {
	id := strings.TrimSpace(rawID)
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node id is required"})
		return
	}

	s.mu.Lock()
	index := -1
	for i, node := range s.nodes {
		if node.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		s.mu.Unlock()
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "node not found"})
		return
	}
	if err := s.deleteNodeRecord(s.nodes[index].dbID); err != nil {
		s.mu.Unlock()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("delete node failed: %v", err)})
		return
	}
	cancel := s.nodeCancels[id]
	delete(s.nodeContexts, id)
	delete(s.nodeCancels, id)
	delete(s.containerNames, id)
	for key := range s.historyCoverage {
		if strings.HasPrefix(key, id+"::") {
			delete(s.historyCoverage, key)
		}
	}
	for key := range s.containerLogs {
		if strings.HasPrefix(key, id+"::") {
			delete(s.containerLogs, key)
		}
	}
	for key := range s.streams {
		if strings.HasPrefix(key, id+":") {
			delete(s.streams, key)
		}
	}
	filteredLogs := s.logs[:0]
	for _, entry := range s.logs {
		if entry.nodeID != id {
			filteredLogs = append(filteredLogs, entry)
		}
	}
	s.logs = filteredLogs
	s.nodes = append(s.nodes[:index], s.nodes[index+1:]...)
	s.historyGeneration++
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseDozzleURL(raw string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", fmt.Errorf("invalid Dozzle URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", fmt.Errorf("Dozzle URL must use http or https")
	}

	parsed.RawQuery = ""
	parsed.Fragment = ""
	path := strings.TrimRight(parsed.Path, "/")
	containerID := ""
	marker := strings.LastIndex(path, "/container/")
	if marker >= 0 {
		containerID = strings.Trim(path[marker+len("/container/"):], "/")
		if containerID == "" || strings.Contains(containerID, "/") {
			return "", "", fmt.Errorf("invalid Dozzle container URL")
		}
		path = strings.TrimRight(path[:marker], "/")
	}
	parsed.Path = path
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), containerID, nil
}

func canonicalNodeURL(raw string) string {
	baseURL, containerID, err := parseDozzleURL(raw)
	if err != nil {
		return strings.TrimRight(strings.TrimSpace(raw), "/")
	}
	if containerID == "" {
		return baseURL
	}
	return baseURL + "/container/" + containerID
}

func (s *server) nodeSnapshot(id string) (Node, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, node := range s.nodes {
		if node.ID == id {
			return node, true
		}
	}
	return Node{}, false
}

func (s *server) setNodeStatus(id, status, nodeError string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.nodes {
		if s.nodes[i].ID != id {
			continue
		}
		s.nodes[i].Status = status
		s.nodes[i].Error = nodeError
		s.nodes[i].Warning = status == "error"
		return
	}
}

func (s *server) setNodeHost(id, hostID, version string, latency int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.nodes {
		if s.nodes[i].ID == id {
			s.nodes[i].hostID = hostID
			s.nodes[i].Version = version
			s.nodes[i].Latency = latency
			return
		}
	}
}

func (s *server) updateNodeContainers(ctx context.Context, id string, containers []dozzleContainer) {
	node, ok := s.nodeSnapshot(id)
	if !ok {
		return
	}

	names := make(map[string]string, len(containers))
	containerInfos := make([]containerInfo, 0, len(containers))
	count := 0
	targetFound := node.containerID == ""
	for _, container := range containers {
		if container.ID == "" {
			continue
		}
		names[container.ID] = container.Name
		containerInfos = append(containerInfos, containerInfo{ID: container.ID, Name: container.Name, State: container.State})
		if container.ID == node.containerID {
			targetFound = true
		}
		if container.State == "running" {
			count++
		}
	}

	status := "connected"
	nodeError := ""
	if !targetFound {
		status = "error"
		nodeError = "container not found in Dozzle"
	}

	s.mu.Lock()
	s.containerNames[id] = names
	for i := range s.nodes {
		if s.nodes[i].ID == id {
			s.nodes[i].Count = count
			s.nodes[i].Containers = containerInfos
			s.nodes[i].Status = status
			s.nodes[i].Error = nodeError
			s.nodes[i].Warning = status == "error"
			break
		}
	}
	s.mu.Unlock()

	if status == "error" {
		return
	}
	for _, container := range containers {
		if container.State != "running" {
			continue
		}
		s.startContainerStreams(ctx, id, node.hostID, container.ID)
	}
}

func (s *server) connectNode(ctx context.Context, id string) {
	node, ok := s.nodeSnapshot(id)
	if !ok {
		return
	}
	hostCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	hostID, version, latency, err := fetchDozzleHost(hostCtx, node.baseURL)
	cancel()
	if err != nil {
		s.setNodeStatus(id, "error", err.Error())
		return
	}
	s.setNodeHost(id, hostID, version, latency)
	s.setNodeStatus(id, "connected", "")

	if node.containerID != "" {
		s.startContainerStreams(ctx, id, hostID, node.containerID)
	}
	go s.consumeDozzleEvents(ctx, id, node.baseURL)
}

func fetchDozzleHost(ctx context.Context, baseURL string) (string, string, int64, error) {
	started := time.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/", nil)
	if err != nil {
		return "", "", 0, err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "", "", 0, fmt.Errorf("Dozzle connection failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", "", 0, fmt.Errorf("Dozzle returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return "", "", 0, fmt.Errorf("read Dozzle configuration failed: %w", err)
	}
	start := strings.Index(string(body), `id="config__json"`)
	if start < 0 {
		return "", "", 0, fmt.Errorf("Dozzle configuration not found")
	}
	start = strings.Index(string(body[start:]), ">")
	if start < 0 {
		return "", "", 0, fmt.Errorf("Dozzle configuration is invalid")
	}
	start += strings.Index(string(body), `id="config__json"`)
	start++
	end := strings.Index(string(body[start:]), "</script>")
	if end < 0 {
		return "", "", 0, fmt.Errorf("Dozzle configuration is incomplete")
	}
	var config dozzleConfig
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(body[start:start+end]))), &config); err != nil {
		return "", "", 0, fmt.Errorf("parse Dozzle configuration failed: %w", err)
	}
	for _, host := range config.Hosts {
		if host.ID != "" && host.Available {
			return host.ID, config.Version, time.Since(started).Milliseconds(), nil
		}
	}
	for _, host := range config.Hosts {
		if host.ID != "" {
			return host.ID, config.Version, time.Since(started).Milliseconds(), nil
		}
	}
	return "", "", 0, fmt.Errorf("Dozzle has no available host")
}

func (s *server) consumeDozzleEvents(ctx context.Context, nodeID, baseURL string) {
	endpoint := baseURL + "/api/events/stream"
	for {
		if ctx.Err() != nil {
			return
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			s.setNodeStatus(nodeID, "error", err.Error())
			return
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.setNodeStatus(nodeID, "error", fmt.Sprintf("Dozzle event stream failed: %v", err))
			if !waitForRetry(ctx, 3*time.Second) {
				return
			}
			continue
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			status := response.StatusCode
			response.Body.Close()
			if ctx.Err() != nil {
				return
			}
			s.setNodeStatus(nodeID, "error", fmt.Sprintf("Dozzle event stream returned HTTP %d", status))
			if !waitForRetry(ctx, 3*time.Second) {
				return
			}
			continue
		}

		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 64*1024), 16<<20)
		returnErr := readDozzleSSE(scanner, func(eventName, data string) {
			if eventName != "containers-changed" {
				return
			}
			var containers []dozzleContainer
			if err := json.Unmarshal([]byte(data), &containers); err != nil {
				s.setNodeStatus(nodeID, "error", fmt.Sprintf("parse Dozzle containers failed: %v", err))
				return
			}
			s.updateNodeContainers(ctx, nodeID, containers)
		})
		response.Body.Close()
		if ctx.Err() != nil {
			return
		}
		if returnErr != nil {
			s.setNodeStatus(nodeID, "error", fmt.Sprintf("Dozzle event stream stopped: %v", returnErr))
		} else {
			s.setNodeStatus(nodeID, "error", "Dozzle event stream closed")
		}
		if !waitForRetry(ctx, 3*time.Second) {
			return
		}
	}
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func readDozzleSSE(scanner *bufio.Scanner, handle func(eventName, data string)) error {
	eventName := ""
	var data strings.Builder
	flush := func() {
		if data.Len() > 0 {
			handle(eventName, data.String())
		}
		eventName = ""
		data.Reset()
	}
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "event:"):
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	flush()
	return scanner.Err()
}

func (s *server) startContainerStreams(ctx context.Context, nodeID, hostID, containerID string) {
	if hostID == "" || containerID == "" {
		return
	}
	if ctx.Err() != nil {
		return
	}
	streamKey := nodeID + ":" + containerID
	s.mu.Lock()
	if _, exists := s.streams[streamKey]; exists {
		s.mu.Unlock()
		return
	}
	node, exists := s.nodeByIDLocked(nodeID)
	if !exists {
		s.mu.Unlock()
		return
	}
	s.streams[streamKey] = struct{}{}
	rangeKey := s.historyRange
	generation := s.historyGeneration
	s.mu.Unlock()
	s.launchHistoryFetch(ctx, node, hostID, containerID, rangeKey, generation)
	go func() {
		defer s.releaseContainerStream(streamKey)
		s.consumeDozzleLogs(ctx, node, hostID, containerID)
	}()
}

func (s *server) releaseContainerStream(streamKey string) {
	s.mu.Lock()
	delete(s.streams, streamKey)
	s.mu.Unlock()
}

func (s *server) fetchDozzleHistory(parent context.Context, node Node, hostID, containerID string, from, to time.Time, generation uint64) {
	s.fetchDozzleHistoryWithCachePolicy(parent, node, hostID, containerID, from, to, generation, false)
}

func (s *server) fetchDozzleHistoryWithCachePolicy(parent context.Context, node Node, hostID, containerID string, from, to time.Time, generation uint64, allowWhenSharedCacheFull bool) {
	// A single unavailable or very busy container must not keep the page in a
	// loading state indefinitely. Normal pagination remains unbounded so the
	// selected node's requested range is complete whenever it fits the cache.
	ctx, cancel := context.WithTimeout(parent, historyFetchTimeout)
	defer cancel()
	completed := false
	defer func() {
		if completed {
			s.markHistoryCoverage(node.ID, containerID, from, generation)
		}
	}()
	pageTo := to
	for {
		// Once the shared cache is full, older pages would immediately be
		// discarded by insertNewestLog. Finish the task instead of keeping the
		// dashboard in a perpetual “loading history” state.
		if !allowWhenSharedCacheFull && s.historyCacheAtCapacity() {
			completed = true
			return
		}
		if allowWhenSharedCacheFull && s.containerHistoryAtCapacity(node.ID, containerID) {
			return
		}
		oldest, count, err := s.fetchDozzleHistoryPage(ctx, node, hostID, containerID, from, pageTo, generation)
		if err != nil {
			// Exhausting the bounded history-query budget is normal for a noisy
			// container; it is not a node failure.
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				s.setNodeStatus(node.ID, "error", err.Error())
			}
			return
		}
		if count == 0 || oldest.IsZero() || !oldest.Before(pageTo) {
			completed = true
			return
		}
		// Dozzle returns at most 500 events for a time window. A full page
		// means there may be more history before its oldest event, so continue
		// with an earlier upper bound and merge the next page into the cache.
		if count < dozzleHistoryPageSize || !oldest.After(from) {
			completed = true
			return
		}
		if !allowWhenSharedCacheFull && s.historyCacheAtCapacity() {
			completed = true
			return
		}
		nextTo := oldest.Add(-time.Millisecond)
		if !nextTo.After(from) || !nextTo.Before(pageTo) {
			completed = true
			return
		}
		pageTo = nextTo
	}
}

func (s *server) containerHistoryAtCapacity(nodeID, containerID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return maxContainerLogs > 0 && len(s.containerLogs[containerLogKey(nodeID, containerID)]) >= maxContainerLogs
}

func (s *server) historyCacheAtCapacity() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return maxStoredLogs > 0 && len(s.logs) >= maxStoredLogs
}

func (s *server) fetchDozzleHistoryPage(ctx context.Context, node Node, hostID, containerID string, from, to time.Time, generation uint64) (time.Time, int, error) {
	query := url.Values{}
	query.Set("stdout", "1")
	query.Set("stderr", "1")
	for _, level := range []string{"debug", "info", "warn", "error", "fatal", "trace", "unknown"} {
		query.Add("levels", level)
	}
	query.Set("from", from.Format(time.RFC3339Nano))
	query.Set("to", to.Format(time.RFC3339Nano))
	endpoint := fmt.Sprintf("%s/api/hosts/%s/containers/%s/logs?%s", node.baseURL, url.PathEscape(hostID), url.PathEscape(containerID), query.Encode())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		s.setNodeStatus(node.ID, "error", err.Error())
		return time.Time{}, 0, err
	}
	request.Header.Set("Accept", "application/x-jsonl")
	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("read Dozzle history failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return time.Time{}, 0, fmt.Errorf("Dozzle history returned HTTP %d", response.StatusCode)
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64*1024), 16<<20)
	oldest := time.Time{}
	count := 0
	for scanner.Scan() {
		if !s.isHistoryGenerationCurrent(generation) {
			return time.Time{}, 0, context.Canceled
		}
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			var event dozzleLogEvent
			if err := json.Unmarshal([]byte(line), &event); err == nil && event.Timestamp > 0 {
				eventTime := time.UnixMilli(event.Timestamp).UTC()
				if oldest.IsZero() || eventTime.Before(oldest) {
					oldest = eventTime
				}
			}
			s.ingestDozzleEventForGeneration(node.ID, []byte(line), generation)
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return time.Time{}, 0, fmt.Errorf("read Dozzle history failed: %w", err)
	}
	return oldest, count, nil
}

// searchDozzleHistory walks the remote API's time cursor until the beginning
// of the chosen range. Unlike normal browsing, it keeps only matching rows,
// so the per-container display cache limit never truncates search results.
func (s *server) searchDozzleHistory(ctx context.Context, node Node, hostID, containerID string, from, to time.Time, needle string) ([]LogEntry, error) {
	pageTo := to
	matches := make([]LogEntry, 0)
	for {
		oldest, count, page, err := s.fetchDozzleHistorySearchPage(ctx, node, hostID, containerID, from, pageTo, needle)
		if err != nil {
			return nil, err
		}
		matches = append(matches, page...)
		if count == 0 || oldest.IsZero() || !oldest.Before(pageTo) || count < dozzleHistoryPageSize || !oldest.After(from) {
			break
		}
		nextTo := oldest.Add(-time.Millisecond)
		if !nextTo.After(from) || !nextTo.Before(pageTo) {
			break
		}
		pageTo = nextTo
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].Timestamp > matches[j].Timestamp })
	return matches, nil
}

func (s *server) fetchDozzleHistorySearchPage(ctx context.Context, node Node, hostID, containerID string, from, to time.Time, needle string) (time.Time, int, []LogEntry, error) {
	query := url.Values{}
	query.Set("stdout", "1")
	query.Set("stderr", "1")
	for _, level := range []string{"debug", "info", "warn", "error", "fatal", "trace", "unknown"} {
		query.Add("levels", level)
	}
	query.Set("from", from.Format(time.RFC3339Nano))
	query.Set("to", to.Format(time.RFC3339Nano))
	endpoint := fmt.Sprintf("%s/api/hosts/%s/containers/%s/logs?%s", node.baseURL, url.PathEscape(hostID), url.PathEscape(containerID), query.Encode())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return time.Time{}, 0, nil, err
	}
	request.Header.Set("Accept", "application/x-jsonl")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return time.Time{}, 0, nil, fmt.Errorf("read Dozzle history failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return time.Time{}, 0, nil, fmt.Errorf("Dozzle history returned HTTP %d", response.StatusCode)
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64*1024), 16<<20)
	oldest := time.Time{}
	count := 0
	entries := make([]LogEntry, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event dozzleLogEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		count++
		if event.Timestamp > 0 {
			eventTime := time.UnixMilli(event.Timestamp).UTC()
			if oldest.IsZero() || eventTime.Before(oldest) {
				oldest = eventTime
			}
		}
		s.ingestDozzleEventWithAppend(node.ID, []byte(line), func(parsed dozzleLogEvent, message string) {
			entry := searchableLogEntry(node, containerID, parsed, message)
			if strings.Contains(normalizeSearchValue(entry.Node+" "+entry.Container+" "+entry.Message), needle) {
				entries = append(entries, entry)
			}
		})
	}
	if err := scanner.Err(); err != nil {
		return time.Time{}, 0, nil, fmt.Errorf("read Dozzle history failed: %w", err)
	}
	return oldest, count, entries, nil
}

func normalizeSearchValue(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func searchableLogEntry(node Node, requestedContainerID string, event dozzleLogEvent, message string) LogEntry {
	containerID := event.Container
	if containerID == "" {
		containerID = requestedContainerID
	}
	containerName := containerID
	for _, container := range node.Containers {
		if container.ID == containerID && container.Name != "" {
			containerName = container.Name
			break
		}
	}
	if containerName == "" {
		containerName = "unknown"
	}
	timestamp := event.Timestamp
	if timestamp <= 0 {
		timestamp = time.Now().UnixMilli()
	}
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(node.ID + "\x00" + containerID + "\x00" + strconv.FormatInt(timestamp, 10) + "\x00" + strconv.FormatUint(uint64(event.ID), 10) + "\x00" + message))
	id := -int64(hash.Sum64() & uint64(0x7fffffffffffffff))
	if id == 0 {
		id = -1
	}
	eventTime := time.UnixMilli(timestamp).Local()
	return LogEntry{ID: id, Date: eventTime.Format("2006/01/02"), Time: eventTime.Format("15:04:05"), Timestamp: timestamp, Level: normalizeLogLevel(event.Level), Node: node.Name, Container: containerName, Message: message, nodeID: node.ID, remoteID: event.ID}
}

func (s *server) isHistoryGenerationCurrent(generation uint64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return generation == s.historyGeneration
}

func normalizeLogRange(value string) (string, bool) {
	switch value {
	case "30m", "5h", "1d", "1w":
		return value, true
	default:
		return "", false
	}
}

func logRangeDuration(value string) (time.Duration, bool) {
	switch value {
	case "30m":
		return 30 * time.Minute, true
	case "5h":
		return 5 * time.Hour, true
	case "1d":
		return 24 * time.Hour, true
	case "1w":
		return 7 * 24 * time.Hour, true
	default:
		return 0, false
	}
}

func (s *server) consumeDozzleLogs(ctx context.Context, node Node, hostID, containerID string) {
	query := url.Values{}
	query.Set("stdout", "1")
	query.Set("stderr", "1")
	for _, level := range []string{"debug", "info", "warn", "error", "fatal", "trace", "unknown"} {
		query.Add("levels", level)
	}
	endpoint := fmt.Sprintf("%s/api/hosts/%s/containers/%s/logs/stream?%s", node.baseURL, url.PathEscape(hostID), url.PathEscape(containerID), query.Encode())
	for {
		if ctx.Err() != nil {
			return
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			s.setNodeStatus(node.ID, "error", err.Error())
			return
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.setNodeStatus(node.ID, "error", fmt.Sprintf("Dozzle log stream failed: %v", err))
			if !waitForRetry(ctx, 3*time.Second) {
				return
			}
			continue
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			status := response.StatusCode
			response.Body.Close()
			if ctx.Err() != nil {
				return
			}
			s.setNodeStatus(node.ID, "error", fmt.Sprintf("Dozzle log stream returned HTTP %d", status))
			if !waitForRetry(ctx, 3*time.Second) {
				return
			}
			continue
		}

		s.setNodeStatus(node.ID, "connected", "")
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 64*1024), 16<<20)
		returnErr := readDozzleSSE(scanner, func(_, data string) {
			if data != "" {
				s.ingestDozzleEvent(node.ID, []byte(data), true)
			}
		})
		response.Body.Close()
		if ctx.Err() != nil {
			return
		}
		if returnErr != nil {
			s.setNodeStatus(node.ID, "error", fmt.Sprintf("Dozzle log stream stopped: %v", returnErr))
		} else {
			s.setNodeStatus(node.ID, "error", "Dozzle log stream closed")
		}
		if !waitForRetry(ctx, 3*time.Second) {
			return
		}
	}
}

func (s *server) nodeByIDLocked(id string) (Node, bool) {
	for _, node := range s.nodes {
		if node.ID == id {
			return node, true
		}
	}
	return Node{}, false
}

func (s *server) ingestDozzleEvent(nodeID string, data []byte, broadcast bool) {
	s.ingestDozzleEventWithAppend(nodeID, data, func(event dozzleLogEvent, message string) {
		s.appendRemoteLog(nodeID, event, message, broadcast)
	})
}

func (s *server) ingestDozzleEventForGeneration(nodeID string, data []byte, generation uint64) {
	s.ingestDozzleEventWithAppend(nodeID, data, func(event dozzleLogEvent, message string) {
		s.appendRemoteLogForGeneration(nodeID, event, message, false, generation)
	})
}

func (s *server) ingestDozzleEventWithAppend(nodeID string, data []byte, appendLog func(dozzleLogEvent, string)) {
	var event dozzleLogEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return
	}
	if event.Type == "group" {
		var lines []dozzleLogLine
		if err := json.Unmarshal(event.Message, &lines); err == nil {
			grouped := make([]string, 0, len(lines))
			for _, line := range lines {
				if message := stripTerminalControlCodes(strings.TrimRight(html.UnescapeString(line.Message), "\r\n")); message != "" {
					grouped = append(grouped, message)
				}
			}
			if len(grouped) > 0 {
				// Dozzle emits one `group` event for multi-line output such as a
				// Java stack trace. Keep it as one log record in source order rather
				// than inserting one separately sortable record per stack-frame line.
				appendLog(event, strings.Join(grouped, "\n"))
				return
			}
		}
		// Dozzle versions can emit a group payload that is not an array of
		// message lines. Preserve its raw text instead of silently losing it.
		fallback := strings.TrimSpace(stripTerminalControlCodes(html.UnescapeString(event.Raw)))
		if fallback == "" {
			var plain string
			if json.Unmarshal(event.Message, &plain) == nil {
				fallback = strings.TrimSpace(stripTerminalControlCodes(html.UnescapeString(plain)))
			}
		}
		if fallback != "" {
			appendLog(event, fallback)
		}
		return
	}

	message := ""
	if len(event.Message) > 0 {
		_ = json.Unmarshal(event.Message, &message)
	}
	if message == "" {
		message = event.Raw
	}
	message = stripTerminalControlCodes(html.UnescapeString(message))
	if message != "" {
		appendLog(event, message)
	}
}

func stripTerminalControlCodes(message string) string {
	var cleaned strings.Builder
	cleaned.Grow(len(message))
	for index := 0; index < len(message); {
		if message[index] == 0x1b {
			index++
			if index >= len(message) {
				break
			}
			switch message[index] {
			case '[':
				// CSI sequences include colors, cursor movement and reset codes.
				index++
				for index < len(message) && (message[index] < 0x40 || message[index] > 0x7e) {
					index++
				}
				if index < len(message) {
					index++
				}
			case ']':
				// OSC sequences end with BEL or ESC followed by a backslash.
				index++
				for index < len(message) {
					if message[index] == 0x07 {
						index++
						break
					}
					if message[index] == 0x1b && index+1 < len(message) && message[index+1] == '\\' {
						index += 2
						break
					}
					index++
				}
			default:
				index++
			}
			continue
		}
		if message[index] < 0x20 && message[index] != '\n' && message[index] != '\r' && message[index] != '\t' {
			index++
			continue
		}
		cleaned.WriteByte(message[index])
		index++
	}
	return cleaned.String()
}

func (s *server) appendRemoteLog(nodeID string, event dozzleLogEvent, message string, broadcast bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendRemoteLogLocked(nodeID, event, message, broadcast)
}

func (s *server) appendRemoteLogForGeneration(nodeID string, event dozzleLogEvent, message string, broadcast bool, generation uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation != s.historyGeneration {
		return
	}
	s.appendRemoteLogLocked(nodeID, event, message, broadcast)
}

func (s *server) appendRemoteLogLocked(nodeID string, event dozzleLogEvent, message string, broadcast bool) {
	node, ok := s.nodeByIDLocked(nodeID)
	if !ok {
		return
	}
	containerName := event.Container
	if names := s.containerNames[nodeID]; names != nil {
		if name := names[event.Container]; name != "" {
			containerName = name
		}
	}
	if containerName == "" {
		containerName = "unknown"
	}
	eventTime := time.Now()
	if event.Timestamp > 0 {
		eventTime = time.UnixMilli(event.Timestamp)
	}
	containerID := event.Container
	if containerID == "" {
		containerID = containerName
	}
	if s.hasDuplicateLogLocked(nodeID, containerID, event, eventTime.UnixMilli(), message) {
		return
	}
	s.nextLogID++
	entry := LogEntry{
		ID:        s.nextLogID,
		Date:      eventTime.Local().Format("2006/01/02"),
		Time:      eventTime.Local().Format("15:04:05"),
		Timestamp: eventTime.UnixMilli(),
		Level:     normalizeLogLevel(event.Level),
		Node:      node.Name,
		Container: containerName,
		Message:   message,
		nodeID:    nodeID,
		remoteID:  event.ID,
	}
	var evictedExisting bool
	s.logs, evictedExisting = insertNewestLog(s.logs, entry, maxStoredLogs)
	if evictedExisting {
		s.evictedLogs++
		s.lastEvictedAt = time.Now().UnixMilli()
	}
	key := containerLogKey(nodeID, containerID)
	s.containerLogs[key], _ = insertNewestLog(s.containerLogs[key], entry, maxContainerLogs)
	s.processed++
	if broadcast {
		for subscriber := range s.subscribers {
			select {
			case subscriber <- entry:
			default:
			}
		}
	}
}

func (s *server) hasDuplicateLogLocked(nodeID, containerID string, event dozzleLogEvent, timestamp int64, message string) bool {
	key := containerLogKey(nodeID, containerID)
	for _, entry := range s.containerLogs[key] {
		if event.Type != "group" && event.ID != 0 && entry.remoteID == event.ID && entry.Timestamp == timestamp && entry.Message == message {
			return true
		}
		if entry.Timestamp == timestamp && entry.Level == normalizeLogLevel(event.Level) && entry.Message == message {
			return true
		}
	}
	return false
}

func insertNewestLog(logs []LogEntry, entry LogEntry, limit int) ([]LogEntry, bool) {
	index := sort.Search(len(logs), func(index int) bool {
		return logs[index].Timestamp <= entry.Timestamp
	})
	logs = append(logs, LogEntry{})
	copy(logs[index+1:], logs[index:])
	logs[index] = entry
	if len(logs) > limit {
		dropped := logs[limit]
		logs = logs[:limit]
		return logs, dropped.ID != entry.ID
	}
	return logs, false
}

func normalizeLogLevel(level string) string {
	switch strings.ToLower(level) {
	case "warn", "warning":
		return "warn"
	case "error", "fatal":
		return "error"
	default:
		return "info"
	}
}

func (s *server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	updates := make(chan LogEntry, 4)
	s.mu.Lock()
	s.subscribers[updates] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subscribers, updates)
		s.mu.Unlock()
		close(updates)
	}()

	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case entry, ok := <-updates:
			if !ok {
				return
			}
			payload, _ := json.Marshal(entry)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		next.ServeHTTP(w, r)
	})
}
