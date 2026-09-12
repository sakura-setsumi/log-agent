package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
)

// The frontend is embedded so the whole dashboard can be shipped as one Go binary.
//
//go:embed index.html styles.css app.js favicon.png
var frontend embed.FS

const (
	defaultMaxStoredLogs = 100000
	maxAllowedStoredLogs = 1000000
	maxContainerLogs     = 5000
)

var maxStoredLogs = defaultMaxStoredLogs

const dozzleHistoryPageSize = 500

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
}

type aiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aiProfile struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
	Type    string `json:"type"`
}

type aiChatRequest struct {
	Messages []aiMessage `json:"messages"`
	Logs     []LogEntry  `json:"logs"`
	Config   *aiProfile  `json:"config,omitempty"`
}

type aiChatResponse struct {
	Choices []struct {
		Message aiMessage `json:"message"`
	} `json:"choices"`
}

type server struct {
	mu                sync.RWMutex
	db                *sql.DB
	nodes             []Node
	logs              []LogEntry
	processed         int
	nextLogID         int64
	historyRange      string
	historyGeneration uint64
	historyPending    int
	rules             map[string]bool
	subscribers       map[chan LogEntry]struct{}
	containerNames    map[string]map[string]string
	containerLogs     map[string][]LogEntry
	historyCoverage   map[string]time.Time
	streams           map[string]struct{}
	nodeContexts      map[string]context.Context
	nodeCancels       map[string]context.CancelFunc
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
	Storage        storageStats    `json:"storage"`
}

type storageStats struct {
	Used     int `json:"used"`
	Capacity int `json:"capacity"`
	Percent  int `json:"percent"`
}

func main() {
	s := newServer()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/bootstrap", s.handleBootstrap)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/logs/container", s.handleContainerLogs)
	mux.HandleFunc("/api/logs/range", s.handleLogRange)
	mux.HandleFunc("/api/rules", s.handleRules)
	mux.HandleFunc("/api/ai/status", s.handleAIStatus)
	mux.HandleFunc("/api/ai/chat", s.handleAIChat)
	mux.HandleFunc("/api/ai/profiles", s.handleAIProfiles)
	mux.HandleFunc("/api/nodes", s.handleNodes)
	mux.HandleFunc("/api/nodes/", s.handleNode)
	mux.HandleFunc("/api/stream", s.handleStream)

	static, err := fs.Sub(frontend, ".")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(static)))

	address := ":8099"
	log.Printf("Log Agent is running at http://localhost%s", address)
	log.Fatal(http.ListenAndServe(address, withSecurityHeaders(mux)))
}

func newServer() *server {
	maxStoredLogs = configuredLogCacheCapacity()
	s := &server{
		nodes:          []Node{},
		logs:           []LogEntry{},
		processed:      0,
		historyRange:   "30m",
		rules:          map[string]bool{"mask": true, "structure": true, "noise": false},
		subscribers:    make(map[chan LogEntry]struct{}),
		containerNames: make(map[string]map[string]string),
		containerLogs:  make(map[string][]LogEntry),
		historyCoverage: make(map[string]time.Time),
		streams:        make(map[string]struct{}),
		nodeContexts:   make(map[string]context.Context),
		nodeCancels:    make(map[string]context.CancelFunc),
	}
	if !databaseConfigured() {
		log.Printf("node database is not configured; using memory-only node storage")
		return s
	}
	db, err := openNodeDatabase()
	if err != nil {
		log.Fatalf("open node database: %v", err)
	}
	s.db = db
	if err := s.loadNodes(); err != nil {
		log.Fatalf("load nodes from database: %v", err)
	}
	if err := s.ensureAIProfileTable(); err != nil { log.Fatalf("ensure model_info table: %v", err) }
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

func (s *server) ensureAIProfileTable() error {
	if s.db == nil { return nil }
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS model_info (id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, name VARCHAR(200) NOT NULL, base_url VARCHAR(500) NOT NULL, api_key VARCHAR(1000) NOT NULL, type TINYINT NOT NULL DEFAULT 2, model_name VARCHAR(200) NOT NULL, create_time DATETIME NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	return err
}

func databaseConfigured() bool {
	return strings.TrimSpace(os.Getenv("DOZZLE_DB_DSN")) != "" || strings.TrimSpace(os.Getenv("DOZZLE_DB_HOST")) != ""
}

func openNodeDatabase() (*sql.DB, error) {
	dsn := strings.TrimSpace(os.Getenv("DOZZLE_DB_DSN"))
	if dsn == "" {
		host := strings.TrimSpace(os.Getenv("DOZZLE_DB_HOST"))
		port := strings.TrimSpace(os.Getenv("DOZZLE_DB_PORT"))
		user := strings.TrimSpace(os.Getenv("DOZZLE_DB_USER"))
		database := strings.TrimSpace(os.Getenv("DOZZLE_DB_NAME"))
		if host == "" || user == "" || database == "" {
			return nil, fmt.Errorf("DOZZLE_DB_HOST, DOZZLE_DB_USER and DOZZLE_DB_NAME are required")
		}
		if port == "" {
			port = "3306"
		}
		config := mysql.Config{
			User:      user,
			Passwd:    os.Getenv("DOZZLE_DB_PASSWORD"),
			Net:       "tcp",
			Addr:      net.JoinHostPort(host, port),
			DBName:    database,
			ParseTime: true,
			Loc:       time.Local,
			Params:    map[string]string{"charset": "utf8mb4"},
		}
		dsn = config.FormatDSN()
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func (s *server) loadNodes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, address, style FROM vps_info ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	loaded := make([]Node, 0)
	for rows.Next() {
		var dbID int64
		var name, address string
		var style sql.NullString
		if err := rows.Scan(&dbID, &name, &address, &style); err != nil {
			return err
		}
		name = strings.TrimSpace(name)
		address = normalizeNodeAddress(address)
		node := Node{
			ID:      nodeIDForDB(dbID),
			Name:    name,
			URL:     address,
			Style:   strings.TrimSpace(style.String),
			Initial: initialForName(name),
			Status:  "connecting",
			dbID:    dbID,
		}
		if node.Style == "" {
			node.Style = "HTTP / WebSocket"
		}
		baseURL, containerID, parseErr := parseDozzleURL(address)
		if parseErr != nil {
			node.Status = "error"
			node.Warning = true
			node.Error = parseErr.Error()
		} else {
			node.baseURL = baseURL
			node.containerID = containerID
		}
		loaded = append(loaded, node)
	}
	if err := rows.Err(); err != nil {
		return err
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
	if s.db == nil {
		return 0, nil
	}
	result, err := s.db.Exec(`INSERT INTO vps_info (name, address, style, create_time) VALUES (?, ?, ?, ?)`, name, address, normalizeNodeStyle(style), time.Now())
	if err != nil {
		return 0, err
	}
	dbID, err := result.LastInsertId()
	if err != nil || dbID <= 0 {
		if err == nil {
			err = fmt.Errorf("database did not return inserted node id")
		}
		return 0, err
	}
	return dbID, nil
}

func (s *server) updateNodeRecord(dbID int64, name, address, style string) error {
	if s.db == nil || dbID <= 0 {
		return nil
	}
	result, err := s.db.Exec(`UPDATE vps_info SET name = ?, address = ?, style = ? WHERE id = ?`, name, address, normalizeNodeStyle(style), dbID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return err
}

func (s *server) deleteNodeRecord(dbID int64) error {
	if s.db == nil || dbID <= 0 {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM vps_info WHERE id = ?`, dbID)
	return err
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
	storage := storageStats{Used: len(s.logs), Capacity: maxStoredLogs}
	if storage.Capacity > 0 {
		storage.Percent = (storage.Used*100 + storage.Capacity - 1) / storage.Capacity
		if storage.Percent > 100 {
			storage.Percent = 100
		}
	}
	writeJSON(w, http.StatusOK, bootstrapResponse{
		Nodes: append([]Node{}, s.nodes...), Logs: logs, Processed: s.processed, Range: s.historyRange,
		HistoryLoading: s.historyPending > 0, Rules: copyRules(s.rules), Storage: storage,
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
	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "rule and enabled are required"})
		return
	}
	for rule := range updates {
		if !isKnownRule(rule) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown rule"})
			return
		}
	}

	s.mu.Lock()
	for rule, enabled := range updates {
		s.rules[rule] = enabled
	}
	rules := copyRules(s.rules)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]map[string]bool{"rules": rules})
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

	s.mu.RLock()
	logs := append([]LogEntry{}, s.containerLogs[containerLogKey(nodeID, containerID)]...)
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, map[string][]LogEntry{"logs": logs})
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "range must be one of 30m, 2h, 1d, 1w"})
		return
	}

	duration, _ := logRangeDuration(rangeKey)
	now := time.Now().UTC()

	s.mu.Lock()
	if s.historyRange == rangeKey {
		s.mu.Unlock()
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "unchanged", "range": rangeKey})
		return
	}
	previousDuration, previousDurationOK := logRangeDuration(s.historyRange)
	loadOlderLogs := !previousDurationOK || duration > previousDuration
	s.historyRange = rangeKey
	s.historyGeneration++
	generation := s.historyGeneration
	// A newer range request invalidates any older in-flight request, but the
	// logs already collected by that request remain useful and are retained.
	s.historyPending = 0
	nodes := append([]Node{}, s.nodes...)
	targets := make([]historyTarget, 0)
	if loadOlderLogs {
		from := now.Add(-duration)
		for _, node := range nodes {
			if node.hostID == "" {
				continue
			}
			containerIDs := s.containerNames[node.ID]
			if len(containerIDs) > 0 {
				for containerID := range containerIDs {
					s.addHistoryTargetLocked(&targets, node, containerID, from, now)
				}
				continue
			}
			if node.containerID != "" {
				s.addHistoryTargetLocked(&targets, node, node.containerID, from, now)
				continue
			}
			for _, container := range node.Containers {
				if container.ID != "" {
					s.addHistoryTargetLocked(&targets, node, container.ID, from, now)
				}
			}
		}
	}
	s.mu.Unlock()

	for _, target := range targets {
		s.launchHistoryFetchWindow(target.ctx, target.node, target.node.hostID, target.containerID, target.from, target.to, generation)
	}
	status := "filtered"
	if loadOlderLogs && len(targets) > 0 {
		status = "loading"
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": status, "range": rangeKey})
}

type historyTarget struct {
	node        Node
	containerID string
	ctx         context.Context
	from        time.Time
	to          time.Time
}

func (s *server) addHistoryTargetLocked(targets *[]historyTarget, node Node, containerID string, from, to time.Time) {
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
		node: node, containerID: containerID, ctx: s.nodeContexts[node.ID], from: from, to: to,
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
	s.launchHistoryFetchWindow(parent, node, hostID, containerID, now.Add(-duration), now, generation)
}

func (s *server) launchHistoryFetchWindow(parent context.Context, node Node, hostID, containerID string, from, to time.Time, generation uint64) {
	if !from.Before(to) {
		return
	}
	s.mu.Lock()
	if generation != s.historyGeneration {
		s.mu.Unlock()
		return
	}
	s.historyPending++
	s.mu.Unlock()

	go func() {
		defer s.completeHistoryFetch(generation)
		s.fetchDozzleHistory(parent, node, hostID, containerID, from, to, generation)
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
	if s.db == nil { writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database is not configured"}); return }
	if r.Method != http.MethodPost && r.Method != http.MethodDelete { writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"}); return }
	if token := strings.TrimSpace(os.Getenv("LOG_AGENT_ADMIN_TOKEN")); token == "" || r.Header.Get("X-Log-Agent-Admin-Token") != token { writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "admin token required"}); return }
	if r.Method == http.MethodDelete {
		var id int64
		if _, err := fmt.Sscanf(r.URL.Query().Get("id"), "%d", &id); err != nil || id <= 0 { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid profile id"}); return }
		if _, err := s.db.Exec(`DELETE FROM model_info WHERE id = ?`, id); err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete AI profile failed"}); return }
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"}); return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var request struct { ID string `json:"id"`; Name string `json:"name"`; BaseURL string `json:"baseURL"`; APIKey string `json:"apiKey"`; Type string `json:"type"`; Models []struct { Name string `json:"name"` } `json:"models"` }
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid AI profile"}); return }
	request.Name, request.BaseURL = strings.TrimSpace(request.Name), strings.TrimRight(strings.TrimSpace(request.BaseURL), "/")
	modelNames := make([]string, 0, len(request.Models)); for _, model := range request.Models { if name := strings.TrimSpace(model.Name); name != "" { modelNames = append(modelNames, name) } }
	profileType := int64(2); if strings.EqualFold(request.Type, "anthropic") { profileType = 1 }
	if request.Name == "" || request.BaseURL == "" || len(modelNames) == 0 { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, base URL and model are required"}); return }
	var id int64; _, scanErr := fmt.Sscanf(request.ID, "ai-profile-db-%d", &id); var err error
	if scanErr == nil && id > 0 {
		if strings.TrimSpace(request.APIKey) == "" { _, err = s.db.Exec(`UPDATE model_info SET name=?, base_url=?, type=?, model_name=? WHERE id=?`, request.Name, request.BaseURL, profileType, strings.Join(modelNames, ","), id) } else { _, err = s.db.Exec(`UPDATE model_info SET name=?, base_url=?, api_key=?, type=?, model_name=? WHERE id=?`, request.Name, request.BaseURL, strings.TrimSpace(request.APIKey), profileType, strings.Join(modelNames, ","), id) }
	} else { var result sql.Result; result, err = s.db.Exec(`INSERT INTO model_info (name, base_url, api_key, type, model_name, create_time) VALUES (?, ?, ?, ?, ?, ?)`, request.Name, request.BaseURL, strings.TrimSpace(request.APIKey), profileType, strings.Join(modelNames, ","), time.Now()); if err == nil { id, err = result.LastInsertId() } }
	if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save AI profile failed"}); return }
	writeJSON(w, http.StatusOK, map[string]string{"id": fmt.Sprintf("ai-profile-db-%d", id)})
}

func (s *server) handleAIChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
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

	profile := configuredAIProfile()
	if request.Config != nil {
		profile = normalizeAIProfile(*request.Config)
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

	payload := struct {
		Model     string      `json:"model"`
		Messages  []aiMessage `json:"messages"`
		MaxTokens int         `json:"max_tokens,omitempty"`
	}{Model: profile.Model, Messages: messages}
	if profile.Type == "anthropic" { payload.MaxTokens = 4096 }
	body, err := json.Marshal(payload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode AI request failed"})
		return
	}

	endpoint := strings.TrimRight(profile.BaseURL, "/") + "/chat/completions"
	if profile.Type == "anthropic" {
		baseURL := strings.TrimRight(profile.BaseURL, "/")
		if strings.HasSuffix(baseURL, "/v1") {
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
	providerResponse, err := (&http.Client{Timeout: 90 * time.Second}).Do(providerRequest)
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
		var response struct { Content []struct { Type string `json:"type"`; Text string `json:"text"` } `json:"content"` }
		if err := json.Unmarshal(providerBody, &response); err != nil { writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid Anthropic response"}); return }
		for _, item := range response.Content { if item.Type == "text" { answer += item.Text } }
	} else {
		var response aiChatResponse
		if err := json.Unmarshal(providerBody, &response); err != nil { writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid AI provider response"}); return }
		if len(response.Choices) > 0 { answer = response.Choices[0].Message.Content }
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
	if profile.Type != "anthropic" { profile.Type = "openai" }
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

func (s *server) handleNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		s.unbindNode(w, r.URL.Query().Get("id"))
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
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
	s.unbindNode(w, id)
}

func (s *server) updateNode(w http.ResponseWriter, r *http.Request, id string) {
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "node id is required"})
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
	ctx, newCancel := context.WithCancel(context.Background())
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
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	completed := false
	defer func() {
		if completed {
			s.markHistoryCoverage(node.ID, containerID, from, generation)
		}
	}()
	pageTo := to
	for {
		oldest, count, err := s.fetchDozzleHistoryPage(ctx, node, hostID, containerID, from, pageTo, generation)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
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
		nextTo := oldest.Add(-time.Millisecond)
		if !nextTo.After(from) || !nextTo.Before(pageTo) {
			completed = true
			return
		}
		pageTo = nextTo
	}
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

func (s *server) isHistoryGenerationCurrent(generation uint64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return generation == s.historyGeneration
}

func normalizeLogRange(value string) (string, bool) {
	switch value {
	case "30m", "2h", "1d", "1w":
		return value, true
	default:
		return "", false
	}
}

func logRangeDuration(value string) (time.Duration, bool) {
	switch value {
	case "30m":
		return 30 * time.Minute, true
	case "2h":
		return 2 * time.Hour, true
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
		if err := json.Unmarshal(event.Message, &lines); err != nil {
			return
		}
		grouped := make([]string, 0, len(lines))
		for _, line := range lines {
			if message := strings.TrimRight(html.UnescapeString(line.Message), "\r\n"); message != "" {
				grouped = append(grouped, message)
			}
		}
		if len(grouped) > 0 {
			// Dozzle emits one `group` event for multi-line output such as a
			// Java stack trace. Keep it as one log record in source order rather
			// than inserting one separately sortable record per stack-frame line.
			appendLog(event, strings.Join(grouped, "\n"))
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
	message = html.UnescapeString(message)
	if message != "" {
		appendLog(event, message)
	}
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
	s.logs = insertNewestLog(s.logs, entry, maxStoredLogs)
	key := containerLogKey(nodeID, containerID)
	s.containerLogs[key] = insertNewestLog(s.containerLogs[key], entry, maxContainerLogs)
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

func insertNewestLog(logs []LogEntry, entry LogEntry, limit int) []LogEntry {
	index := sort.Search(len(logs), func(index int) bool {
		return logs[index].Timestamp <= entry.Timestamp
	})
	logs = append(logs, LogEntry{})
	copy(logs[index+1:], logs[index:])
	logs[index] = entry
	if len(logs) > limit {
		logs = logs[:limit]
	}
	return logs
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
