package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/gif"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCommandConnectionTest(t *testing.T) {
	originalDial := commandConnectionDial
	defer func() { commandConnectionDial = originalDial }()
	var received commandConnectionRequest
	commandConnectionDial = func(request commandConnectionRequest) error { received = request; return nil }
	requestBody, _ := json.Marshal(commandConnectionRequest{Host: "127.0.0.1", Port: "22", User: "deploy", Auth: "key", Secret: "private-key", Fingerprint: "SHA256:test"})
	response := httptest.NewRecorder()
	(&server{}).handleCommandConnectionTest(response, httptest.NewRequest(http.MethodPost, "/api/commands/test", bytes.NewReader(requestBody)))
	if response.Code != http.StatusOK {
		t.Fatalf("expected successful connection test, got %d: %s", response.Code, response.Body.String())
	}
	if received.User != "deploy" || received.Secret != "private-key" || received.Fingerprint != "SHA256:test" {
		t.Fatalf("expected SSH credentials and fingerprint to reach the dialer, got %#v", received)
	}
}

func TestValidateRemoteCommandPolicy(t *testing.T) {
	allowed := []string{`mv "images (1).jpg" image.jpg`, `rm -f old.log`, `docker stop api`, `systemctl stop log-agent.service`, `sudo -n systemctl stop log-agent.service`, `df -h`}
	for _, command := range allowed {
		if err := validateRemoteCommand(command, "opc"); err != nil {
			t.Errorf("expected command to be allowed %q: %v", command, err)
		}
	}
	rejected := []string{`mv file /etc/app.conf`, `sh -c "rm file"`, `echo value > /etc/app.conf`, `rm file; systemctl stop sshd`}
	for _, command := range rejected {
		if err := validateRemoteCommand(command, "opc"); err == nil {
			t.Errorf("expected command to be rejected: %q", command)
		}
	}
}

func TestCommandExecuteHandler(t *testing.T) {
	originalRun := commandExecutionRun
	defer func() { commandExecutionRun = originalRun }()
	commandExecutionRun = func(request commandExecutionRequest) (string, error) {
		if request.Command != `mv "images (1).jpg" image.jpg` {
			t.Fatalf("unexpected command: %q", request.Command)
		}
		return "done", nil
	}
	body, _ := json.Marshal(commandExecutionRequest{Host: "127.0.0.1", Port: "22", User: "opc", Auth: "key", Secret: "private-key", Fingerprint: "SHA256:test", Command: `mv "images (1).jpg" image.jpg`})
	response := httptest.NewRecorder()
	(&server{}).handleCommandExecute(response, httptest.NewRequest(http.MethodPost, "/api/commands/execute", bytes.NewReader(body)))
	if response.Code != http.StatusOK {
		t.Fatalf("expected command execution success, got %d: %s", response.Code, response.Body.String())
	}
}

func TestEmbeddedLoadingAnimationKeepsFullLoop(t *testing.T) {
	data, err := frontend.ReadFile("loading.gif")
	if err != nil {
		t.Fatalf("read embedded loading animation: %v", err)
	}
	animation, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode embedded loading animation: %v", err)
	}
	if len(animation.Image) != 41 {
		t.Fatalf("expected the complete 41-frame loading loop, got %d frames", len(animation.Image))
	}
	totalDelay := 0
	for _, delay := range animation.Delay {
		totalDelay += delay
	}
	if totalDelay < 400 {
		t.Fatalf("loading loop was truncated: duration=%dms", totalDelay*10)
	}
}

func TestNewServerStartsWithFileBackedStorage(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("LOG_AGENT_CONFIG_DIR", configRoot)
	resetConfigDirCache(t)

	s := newServer()
	if s.store == nil {
		t.Fatal("expected a file-backed config store to be created")
	}
	if s.nodes == nil {
		t.Fatal("expected an initialized node slice on a fresh install")
	}
	if len(s.nodes) != 0 {
		t.Fatalf("expected no nodes on a fresh install, got %d", len(s.nodes))
	}
	if filepath.Dir(nodesFilePath()) != configRoot {
		t.Fatalf("node config path = %s, want it under %s", nodesFilePath(), configRoot)
	}
	if filepath.Dir(modelsFilePath()) != configRoot {
		t.Fatalf("model config path = %s, want it under %s", modelsFilePath(), configRoot)
	}
}

// The JSON store must survive a hand-edited file: invalid rows are dropped and
// IDs are reassigned so the rest of the app never sees duplicates or gaps.
func TestConfigStoreSanitizesHandEditedFiles(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("LOG_AGENT_CONFIG_DIR", configRoot)
	resetConfigDirCache(t)

	raw := `[
  {"id": 7, "name": "kept", "address": "http://127.0.0.1:8080"},
  {"id": 7, "name": "duplicate id", "address": "http://127.0.0.1:8081"},
  {"id": 3, "name": "", "address": "http://127.0.0.1:8082"},
  {"id": 4, "name": "no address", "address": ""}
]`
	if err := os.WriteFile(nodesFilePath(), []byte(raw), 0600); err != nil {
		t.Fatalf("write hand-edited nodes file: %v", err)
	}

	store := newConfigStore()
	nodes, err := store.listNodes()
	if err != nil {
		t.Fatalf("list nodes from hand-edited file: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 valid nodes, got %d: %+v", len(nodes), nodes)
	}
	seen := make(map[int64]bool, len(nodes))
	for _, node := range nodes {
		if seen[node.ID] {
			t.Fatalf("duplicate node id %d survived sanitizing", node.ID)
		}
		seen[node.ID] = true
		if node.Name == "" || node.Address == "" {
			t.Fatalf("incomplete node survived sanitizing: %+v", node)
		}
	}
}

// Nodes and models live in separate files, so a node-only write must not
// disturb the model catalog.
func TestConfigStoreKeepsNodesAndModelsIndependent(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("LOG_AGENT_CONFIG_DIR", configRoot)
	resetConfigDirCache(t)

	store := newConfigStore()
	nodeID, err := store.insertNode("local", "http://127.0.0.1:8099", "dozzle")
	if err != nil {
		t.Fatalf("insert node: %v", err)
	}
	if nodeID <= 0 {
		t.Fatalf("insert node returned id %d", nodeID)
	}
	modelID, err := store.upsertModel(0, "example", "https://api.example.com", "sk-secret", 2, []string{"gpt-4o-mini"})
	if err != nil {
		t.Fatalf("upsert model: %v", err)
	}

	nodes, err := store.listNodes()
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	models, err := store.listModels()
	if err != nil {
		t.Fatalf("list models: %v", err)
	}
	if len(nodes) != 1 || len(models) != 1 {
		t.Fatalf("expected 1 node and 1 model, got %d and %d", len(nodes), len(models))
	}

	// Editing a provider without re-sending the key must not wipe it.
	if _, err := store.upsertModel(modelID, "example renamed", "https://api.example.com", "", 2, []string{"gpt-4o-mini", "gpt-4o"}); err != nil {
		t.Fatalf("update model: %v", err)
	}
	record, err := store.findModel(modelID)
	if err != nil {
		t.Fatalf("find model after update: %v", err)
	}
	if record.APIKey != "sk-secret" {
		t.Fatalf("stored api key = %q, want it preserved across an edit", record.APIKey)
	}
	if record.Name != "example renamed" {
		t.Fatalf("stored name = %q, want the update applied", record.Name)
	}

	if err := store.deleteNode(nodeID); err != nil {
		t.Fatalf("delete node: %v", err)
	}
	models, err = store.listModels()
	if err != nil {
		t.Fatalf("list models after node delete: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("deleting a node changed the model catalog: %d models remain", len(models))
	}
}

// Config files hold API keys, so they must not be group/world readable.
func TestConfigStoreWritesPrivateFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not report POSIX file modes")
	}
	configRoot := t.TempDir()
	t.Setenv("LOG_AGENT_CONFIG_DIR", configRoot)
	resetConfigDirCache(t)

	store := newConfigStore()
	if _, err := store.upsertModel(0, "example", "https://api.example.com", "sk-secret", 2, []string{"gpt-4o-mini"}); err != nil {
		t.Fatalf("upsert model: %v", err)
	}
	info, err := os.Stat(modelsFilePath())
	if err != nil {
		t.Fatalf("stat models file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("models file mode = %o, want 600", perm)
	}
}

func TestConfigStoreReplaceAllResolvesIncomingIDs(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("LOG_AGENT_CONFIG_DIR", configRoot)
	resetConfigDirCache(t)

	store := newConfigStore()
	if _, err := store.insertNode("old", "http://127.0.0.1:8080", "dozzle"); err != nil {
		t.Fatalf("insert node: %v", err)
	}

	// An import file may omit ids or repeat them; replaceAll owns the final set.
	if err := store.replaceAll(
		[]storedNode{
			{Name: "a", Address: "http://127.0.0.1:9001"},
			{ID: 42, Name: "b", Address: "http://127.0.0.1:9002"},
			{ID: 42, Name: "c", Address: "http://127.0.0.1:9003"},
		},
		[]storedModel{{Name: "p", BaseURL: "https://api.example.com", ModelName: "m"}},
	); err != nil {
		t.Fatalf("replace all: %v", err)
	}

	nodes, err := store.listNodes()
	if err != nil {
		t.Fatalf("list nodes after replace: %v", err)
	}
	if len(nodes) != 3 {
		t.Fatalf("expected 3 imported nodes, got %d: %+v", len(nodes), nodes)
	}
	seen := make(map[int64]bool, len(nodes))
	for _, node := range nodes {
		if seen[node.ID] {
			t.Fatalf("duplicate node id %d after import", node.ID)
		}
		seen[node.ID] = true
	}
	models, err := store.listModels()
	if err != nil {
		t.Fatalf("list models after replace: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 imported model, got %d", len(models))
	}
}

// configDir() memoises its result, so tests that change the environment must
// reset the cache between cases. Production code never needs this: the cache
// is deliberately write-once so a running process cannot change its target.
//
// The struct is cleared through a pointer rather than copied, because
// sync.Once must not be copied after first use.
func resetConfigDirCache(t *testing.T) {
	t.Helper()
	*configDirCache() = configDirState{}
	t.Cleanup(func() { *configDirCache() = configDirState{} })
}

func TestConfigDirPrefersExplicitOverride(t *testing.T) {
	override := t.TempDir()
	t.Setenv("LOG_AGENT_CONFIG_DIR", override)
	resetConfigDirCache(t)
	if got := configDir(); got != override {
		t.Fatalf("configDir() = %s, want the LOG_AGENT_CONFIG_DIR override %s", got, override)
	}
}

// The default location is the "data" folder beside the executable, so an
// install stays self-contained and the whole folder can be copied elsewhere.
func TestConfigDirDefaultsToDataFolderBesideExecutable(t *testing.T) {
	t.Setenv("LOG_AGENT_CONFIG_DIR", "")
	resetConfigDirCache(t)

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	got := configDir()
	want := filepath.Join(filepath.Dir(executable), "data")
	if got != want {
		// A read-only build directory legitimately falls back; accept that but
		// make sure it did not silently resolve somewhere relative.
		if filepath.IsAbs(got) && strings.Contains(got, "logAgent") {
			t.Skipf("executable directory is not writable; fell back to %s", got)
		}
		t.Fatalf("configDir() = %s, want %s", got, want)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("configDir() = %s, want an absolute path", got)
	}
}

func TestIsWritableDirRejectsUncreatablePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows path semantics make an uncreatable path hard to express portably")
	}
	// A path under a regular file can never be created.
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	if isWritableDir(filepath.Join(file, "child")) {
		t.Fatal("isWritableDir accepted a path under an existing file")
	}
}

// validateConfigDir exists to stop `go run`, whose executable lives in Go's
// temp build directory and is deleted on exit along with any configuration
// written beside it. Losing a user's nodes that way is silent, so starting is
// refused instead.
//
// Telling `go run` apart from `go test` matters: both build into the same
// go-build<digits> tree, so the check cannot be "is it in temp". The test
// toolchain names its artifact with a .test suffix, which is what keeps this
// guard from refusing to run the test suite that covers it.
func TestIsGoRunExecutable(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "go run builds dozzle-ops.exe",
			path: filepath.Join(os.TempDir(), "go-build1910004526", "b001", "exe", "dozzle-ops.exe"),
			want: true,
		},
		{
			name: "go test builds dozzle-ops.test.exe",
			path: filepath.Join(os.TempDir(), "go-build999", "b001", "dozzle-ops.test.exe"),
			want: false,
		},
		{
			name: "a normal install is never blocked",
			path: filepath.Join(string(filepath.Separator)+"opt", "dozzle-ops.exe"),
			want: false,
		},
		{
			name: "a non-numeric go-build suffix is not Go's directory",
			path: filepath.Join(string(filepath.Separator)+"srv", "go-buildX", "dozzle-ops.exe"),
			want: false,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := isGoRunExecutable(testCase.path); got != testCase.want {
				t.Errorf("isGoRunExecutable(%q) = %t, want %t", testCase.path, got, testCase.want)
			}
		})
	}
}

// An explicit LOG_AGENT_CONFIG_DIR is the documented escape hatch, so the guard
// must stand down even when the executable is in Go's build directory.
func TestValidateConfigDirHonoursExplicitOverride(t *testing.T) {
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	if err := validateConfigDir(); err != nil {
		t.Fatalf("validateConfigDir ignored the explicit override: %v", err)
	}
}

// A normal install must never be blocked: this binary lives outside the temp
// directory, so validateConfigDir has nothing to complain about.
func TestValidateConfigDirAcceptsNormalInstall(t *testing.T) {
	t.Setenv("LOG_AGENT_CONFIG_DIR", "")
	if err := validateConfigDir(); err != nil {
		t.Fatalf("validateConfigDir rejected a non-temp executable: %v", err)
	}
}

// withinDir decides whether a path sits under a directory, which is what makes
// the temp-directory check work without a false positive on a normal install
// (e.g. C:\tmp-like prefixes must not match C:\tmpxyz).
func TestWithinDir(t *testing.T) {
	root := filepath.Join(string(filepath.Separator)+"base", "tmp")
	cases := []struct {
		path string
		want bool
	}{
		{path: filepath.Join(root, "a", "b"), want: true},
		{path: root, want: true},
		{path: filepath.Join(root+"x", "a"), want: false},
		{path: filepath.Join(string(filepath.Separator)+"other", "a"), want: false},
	}
	for _, testCase := range cases {
		if got := withinDir(testCase.path, root); got != testCase.want {
			t.Errorf("withinDir(%q, %q) = %t, want %t", testCase.path, root, got, testCase.want)
		}
	}
}

func TestConfigDirResultIsStableAcrossCalls(t *testing.T) {
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	resetConfigDirCache(t)
	first := configDir()
	// Mutating the environment after the first call must not move the target,
	// otherwise a running process could start writing to a different file.
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	if second := configDir(); second != first {
		t.Fatalf("configDir() changed from %s to %s without a restart", first, second)
	}
}

func TestListenAddressResolution(t *testing.T) {
	cases := []struct {
		name    string
		port    string
		envPort string
		want    string
	}{
		{name: "default", want: ":8099"},
		{name: "LOG_AGENT_PORT wins", port: "7777", envPort: "6666", want: ":7777"},
		{name: "PORT fallback", envPort: "6666", want: ":6666"},
		{name: "leading colon tolerated", port: ":7777", want: ":7777"},
		{name: "invalid falls back", port: "not-a-port", want: ":8099"},
		{name: "out of range falls back", port: "70000", want: ":8099"},
		{name: "zero falls back", port: "0", want: ":8099"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("LOG_AGENT_PORT", testCase.port)
			t.Setenv("PORT", testCase.envPort)
			if got := listenAddress(); got != testCase.want {
				t.Fatalf("listenAddress() = %s, want %s", got, testCase.want)
			}
		})
	}
}

func TestRuleOrderUpdateValidatesAndReturnsNewOrder(t *testing.T) {
	s := &server{rules: map[string]bool{"mask": true, "structure": true, "noise": false}, ruleOrder: defaultRuleOrder()}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/rules", bytes.NewBufferString(`{"order":["noise","mask","structure"]}`))
	request.Header.Set("Content-Type", "application/json")
	s.handleRules(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected successful rule order update, got %d", response.Code)
	}
	var payload struct {
		Order []string `json:"order"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode rule order response: %v", err)
	}
	if got, want := fmt.Sprint(payload.Order), "[noise mask structure]"; got != want {
		t.Fatalf("rule order = %s, want %s", got, want)
	}

	invalidResponse := httptest.NewRecorder()
	invalidRequest := httptest.NewRequest(http.MethodPut, "/api/rules", bytes.NewBufferString(`{"order":["mask","mask","noise"]}`))
	s.handleRules(invalidResponse, invalidRequest)
	if invalidResponse.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid duplicate order to be rejected, got %d", invalidResponse.Code)
	}
}

func TestStripTerminalControlCodes(t *testing.T) {
	input := "\x1b[38;5;160mERROR\x1b[0m\nplain\x1b]0;title\x07"
	want := "ERROR\nplain"
	if got := stripTerminalControlCodes(input); got != want {
		t.Fatalf("stripTerminalControlCodes() = %q, want %q", got, want)
	}
}

func TestAIAttachmentsBuildMultimodalProviderMessage(t *testing.T) {
	textData := base64.StdEncoding.EncodeToString([]byte("trace_id=abc"))
	imageData := base64.StdEncoding.EncodeToString([]byte("image-bytes"))
	attachments, err := normalizeAIAttachments([]aiAttachment{
		{Name: "trace.log", Type: "text/plain", Data: "data:text/plain;base64," + textData},
		{Name: "error.png", Type: "image/png", Data: "data:image/png;base64," + imageData},
	})
	if err != nil {
		t.Fatalf("normalize attachments: %v", err)
	}
	messages := openAIProviderMessages([]aiMessage{{Role: "user", Content: "请分析附件"}}, attachments)
	if len(messages) != 1 || messages[0].Role != "user" {
		t.Fatalf("unexpected provider messages: %#v", messages)
	}
	parts, ok := messages[0].Content.([]map[string]any)
	if !ok || len(parts) != 2 {
		t.Fatalf("expected text and image content parts, got %#v", messages[0].Content)
	}
	if parts[0]["type"] != "text" || !strings.Contains(parts[0]["text"].(string), "trace_id=abc") {
		t.Fatalf("text attachment content missing: %#v", parts[0])
	}
	if parts[1]["type"] != "image_url" {
		t.Fatalf("image attachment content missing: %#v", parts[1])
	}
}

func TestLogRangeReportsHistoryLoadingUntilAllTargetsFinish(t *testing.T) {
	releaseHistory := make(chan struct{})
	dozzle := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/hosts/host-1/containers/container-1/logs" {
			http.NotFound(w, r)
			return
		}
		<-releaseHistory
		w.Header().Set("Content-Type", "application/x-jsonl")
		_, _ = fmt.Fprintln(w, `{"t":"single","m":"older log","rm":"older log","ts":1700000000000,"id":1,"l":"info","c":"container-1"}`)
	}))
	defer dozzle.Close()

	s := &server{
		nodes:          []Node{{ID: "node-1", Name: "node", baseURL: dozzle.URL, hostID: "host-1", Containers: []containerInfo{{ID: "container-1", Name: "api", State: "running"}}}},
		logs:           []LogEntry{},
		historyRange:   "30m",
		rules:          map[string]bool{},
		containerNames: map[string]map[string]string{},
		containerLogs:  map[string][]LogEntry{},
		nodeContexts:   map[string]context.Context{"node-1": context.Background()},
		subscribers:    map[chan LogEntry]struct{}{},
		streams:        map[string]struct{}{},
		nodeCancels:    map[string]context.CancelFunc{},
	}

	request := httptest.NewRequest(http.MethodPost, "/api/logs/range?range=1w", nil)
	rangeResponse := httptest.NewRecorder()
	s.handleLogRange(rangeResponse, request)
	if rangeResponse.Code != http.StatusAccepted {
		t.Fatalf("expected accepted range response, got %d", rangeResponse.Code)
	}

	loadingResponse := httptest.NewRecorder()
	s.handleBootstrap(loadingResponse, httptest.NewRequest(http.MethodGet, "/api/bootstrap", nil))
	var loading bootstrapResponse
	if err := json.Unmarshal(loadingResponse.Body.Bytes(), &loading); err != nil {
		t.Fatalf("decode loading bootstrap: %v", err)
	}
	if !loading.HistoryLoading {
		t.Fatal("expected bootstrap to report history loading while the Dozzle request is blocked")
	}

	close(releaseHistory)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !s.historyLoading() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if s.historyLoading() {
		t.Fatal("history loading did not finish")
	}
	if len(s.logs) != 1 || s.logs[0].Message != "older log" {
		t.Fatalf("expected the completed historical log to be stored, got %#v", s.logs)
	}
}

func TestLogRangeKeepsCacheAndLoadsOnlyMissingOlderInterval(t *testing.T) {
	existingTimestamp := time.Now().Add(-30 * time.Minute).UnixMilli()
	var requestedFrom, requestedTo time.Time
	dozzle := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		requestedFrom, err = time.Parse(time.RFC3339Nano, r.URL.Query().Get("from"))
		if err != nil {
			t.Errorf("parse requested from: %v", err)
		}
		requestedTo, err = time.Parse(time.RFC3339Nano, r.URL.Query().Get("to"))
		if err != nil {
			t.Errorf("parse requested to: %v", err)
		}
		w.Header().Set("Content-Type", "application/x-jsonl")
		_, _ = fmt.Fprintf(w, `{"t":"single","m":"older log","rm":"older log","ts":%d,"id":2,"l":"info","c":"container-1"}`+"\n", time.Now().Add(-12*time.Hour).UnixMilli())
	}))
	defer dozzle.Close()

	existing := LogEntry{ID: 1, Timestamp: existingTimestamp, Message: "current log", Level: "info", Node: "node", Container: "api", nodeID: "node-1"}
	s := &server{
		nodes:          []Node{{ID: "node-1", Name: "node", baseURL: dozzle.URL, hostID: "host-1", Containers: []containerInfo{{ID: "container-1", Name: "api", State: "running"}}}},
		logs:           []LogEntry{existing},
		processed:      7,
		nextLogID:      1,
		historyRange:   "30m",
		rules:          map[string]bool{},
		containerNames: map[string]map[string]string{"node-1": {"container-1": "api"}},
		containerLogs:  map[string][]LogEntry{containerLogKey("node-1", "container-1"): {existing}},
		nodeContexts:   map[string]context.Context{"node-1": context.Background()},
		subscribers:    map[chan LogEntry]struct{}{},
		streams:        map[string]struct{}{},
		nodeCancels:    map[string]context.CancelFunc{},
	}

	response := httptest.NewRecorder()
	s.handleLogRange(response, httptest.NewRequest(http.MethodPost, "/api/logs/range?range=1d", nil))
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected accepted range response, got %d", response.Code)
	}
	if len(s.logs) != 1 || s.logs[0].Message != "current log" {
		t.Fatalf("range change should retain existing cache, got %#v", s.logs)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && s.historyLoading() {
		time.Sleep(10 * time.Millisecond)
	}
	if s.historyLoading() {
		t.Fatal("incremental history loading did not finish")
	}
	currentFound, olderFound := false, false
	for _, log := range s.logs {
		currentFound = currentFound || log.Message == "current log"
		olderFound = olderFound || log.Message == "older log"
	}
	if len(s.logs) != 2 || !currentFound || !olderFound || s.processed != 8 {
		t.Fatalf("expected retained and incrementally loaded logs, got logs=%#v processed=%d", s.logs, s.processed)
	}
	if requestedFrom.After(time.Now().Add(-23*time.Hour)) || requestedFrom.Before(time.Now().Add(-25*time.Hour)) {
		t.Fatalf("expected a one-day lower bound, got %s", requestedFrom)
	}
	if requestedTo.After(time.Now().Add(-29 * time.Minute)) {
		t.Fatalf("expected incremental upper bound near the cached earliest log, got %s", requestedTo)
	}
}

func TestLogRangeLoadsOnlyRequestedNodeScope(t *testing.T) {
	requests := make(map[string]int)
	dozzle := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests[r.URL.Path]++
		w.Header().Set("Content-Type", "application/x-jsonl")
		_, _ = fmt.Fprintln(w, `{"t":"single","m":"history","rm":"history","ts":1700000000000,"id":1,"l":"info","c":"container"}`)
	}))
	defer dozzle.Close()

	s := &server{
		nodes: []Node{
			{ID: "node-1", Name: "one", baseURL: dozzle.URL, hostID: "host-1", Containers: []containerInfo{{ID: "container-1", Name: "one", State: "running"}}},
			{ID: "node-2", Name: "two", baseURL: dozzle.URL, hostID: "host-2", Containers: []containerInfo{{ID: "container-2", Name: "two", State: "running"}}},
		},
		historyRange:   "30m",
		rules:          map[string]bool{},
		containerNames: map[string]map[string]string{},
		containerLogs:  map[string][]LogEntry{},
		nodeContexts:   map[string]context.Context{"node-1": context.Background(), "node-2": context.Background()},
		subscribers:    map[chan LogEntry]struct{}{},
		streams:        map[string]struct{}{},
		nodeCancels:    map[string]context.CancelFunc{},
	}

	response := httptest.NewRecorder()
	s.handleLogRange(response, httptest.NewRequest(http.MethodPost, "/api/logs/range?range=1d&node=node-1", nil))
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected accepted range response, got %d", response.Code)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && s.historyLoading() {
		time.Sleep(10 * time.Millisecond)
	}
	if s.historyLoading() {
		t.Fatal("scoped history loading did not finish")
	}
	if requests["/api/hosts/host-1/containers/container-1/logs"] != 1 {
		t.Fatalf("expected selected node history request, got %#v", requests)
	}
	if requests["/api/hosts/host-2/containers/container-2/logs"] != 0 {
		t.Fatalf("unselected node must not be queried, got %#v", requests)
	}
}

func TestLogRangeLoadsOnlyRequestedContainerWhenSharedCacheIsFull(t *testing.T) {
	previousCapacity := maxStoredLogs
	maxStoredLogs = 1
	defer func() { maxStoredLogs = previousCapacity }()

	requests := make(map[string]int)
	dozzle := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests[r.URL.Path]++
		w.Header().Set("Content-Type", "application/x-jsonl")
		_, _ = fmt.Fprintln(w, `{"t":"single","m":"selected older log","rm":"selected older log","ts":1700000000000,"id":2,"l":"info","c":"container-1"}`)
	}))
	defer dozzle.Close()

	s := &server{
		nodes:                 []Node{{ID: "node-1", Name: "one", baseURL: dozzle.URL, hostID: "host-1", Containers: []containerInfo{{ID: "container-1", Name: "one", State: "running"}, {ID: "container-2", Name: "two", State: "running"}}}},
		logs:                  []LogEntry{{ID: 1, Timestamp: time.Now().UnixMilli(), Message: "cached"}},
		nextLogID:             1,
		historyRange:          "30m",
		rules:                 map[string]bool{},
		containerNames:        map[string]map[string]string{"node-1": {"container-1": "one", "container-2": "two"}},
		containerLogs:         map[string][]LogEntry{},
		nodeContexts:          map[string]context.Context{"node-1": context.Background()},
		subscribers:           map[chan LogEntry]struct{}{},
		streams:               map[string]struct{}{},
		nodeCancels:           map[string]context.CancelFunc{},
		historyLoads:          map[string]struct{}{},
		historyLoadGeneration: map[string]uint64{},
	}

	response := httptest.NewRecorder()
	s.handleLogRange(response, httptest.NewRequest(http.MethodPost, "/api/logs/range?range=1d&node=node-1&container=node-1%3A%3Acontainer-1", nil))
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected accepted range response, got %d", response.Code)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && s.historyLoading() {
		time.Sleep(10 * time.Millisecond)
	}
	if s.historyLoading() {
		t.Fatal("selected container history loading did not finish")
	}
	if requests["/api/hosts/host-1/containers/container-1/logs"] != 1 || requests["/api/hosts/host-1/containers/container-2/logs"] != 0 {
		t.Fatalf("expected only selected container to be queried, got %#v", requests)
	}
	loaded := s.containerLogs[containerLogKey("node-1", "container-1")]
	if len(loaded) != 1 || loaded[0].Message != "selected older log" {
		t.Fatalf("expected selected history to bypass full shared cache, got %#v", loaded)
	}
}

func TestGroupedDozzleLogFallsBackToRawMessage(t *testing.T) {
	s := &server{
		nodes:          []Node{{ID: "node-1", Name: "node"}},
		rules:          map[string]bool{},
		containerNames: map[string]map[string]string{"node-1": {"container-1": "api"}},
		containerLogs:  map[string][]LogEntry{},
		subscribers:    map[chan LogEntry]struct{}{},
	}
	s.ingestDozzleEvent("node-1", []byte(`{"t":"group","m":{"unexpected":true},"rm":"failed group message","ts":1700000000000,"id":8,"l":"info","c":"container-1"}`), false)
	if len(s.logs) != 1 || s.logs[0].Message != "failed group message" {
		t.Fatalf("expected group fallback to preserve raw message, got %#v", s.logs)
	}
}

func TestContainerLogsExpandsExistingCacheToRequestedRange(t *testing.T) {
	dozzle := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-jsonl")
		_, _ = fmt.Fprintln(w, `{"t":"single","m":"older selected log","rm":"older selected log","ts":1700000000000,"id":2,"l":"info","c":"container-1"}`)
	}))
	defer dozzle.Close()

	current := LogEntry{ID: 1, Timestamp: time.Now().UnixMilli(), Message: "current selected log", Node: "node", Container: "api", nodeID: "node-1"}
	s := &server{
		nodes:                 []Node{{ID: "node-1", Name: "node", baseURL: dozzle.URL, hostID: "host-1"}},
		logs:                  []LogEntry{current},
		nextLogID:             1,
		historyRange:          "1w",
		rules:                 map[string]bool{},
		containerNames:        map[string]map[string]string{"node-1": {"container-1": "api"}},
		containerLogs:         map[string][]LogEntry{containerLogKey("node-1", "container-1"): {current}},
		nodeContexts:          map[string]context.Context{"node-1": context.Background()},
		subscribers:           map[chan LogEntry]struct{}{},
		streams:               map[string]struct{}{},
		nodeCancels:           map[string]context.CancelFunc{},
		historyLoads:          map[string]struct{}{},
		historyLoadGeneration: map[string]uint64{},
	}

	response := httptest.NewRecorder()
	s.handleContainerLogs(response, httptest.NewRequest(http.MethodGet, "/api/logs/container?node=node-1&container=container-1&range=1w", nil))
	var payload containerLogsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode container response: %v", err)
	}
	if !payload.Loading {
		t.Fatal("expected existing short cache to start loading the requested week")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && s.historyLoading() {
		time.Sleep(10 * time.Millisecond)
	}
	if s.historyLoading() {
		t.Fatal("container range expansion did not finish")
	}
	if got := s.containerLogs[containerLogKey("node-1", "container-1")]; len(got) != 2 {
		t.Fatalf("expected both cached and older container logs, got %#v", got)
	}
}

func TestLogRangeNarrowsWithoutClearingCache(t *testing.T) {
	now := time.Now().UnixMilli()
	existing := LogEntry{ID: 1, Timestamp: now, Message: "current log", Level: "info", Node: "node", Container: "api", nodeID: "node-1"}
	s := &server{
		nodes:          []Node{{ID: "node-1", Name: "node"}},
		logs:           []LogEntry{existing},
		processed:      7,
		historyRange:   "1w",
		rules:          map[string]bool{},
		containerNames: map[string]map[string]string{},
		containerLogs:  map[string][]LogEntry{},
		nodeContexts:   map[string]context.Context{},
		subscribers:    map[chan LogEntry]struct{}{},
		streams:        map[string]struct{}{},
		nodeCancels:    map[string]context.CancelFunc{},
	}

	response := httptest.NewRecorder()
	s.handleLogRange(response, httptest.NewRequest(http.MethodPost, "/api/logs/range?range=30m", nil))
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected accepted range response, got %d", response.Code)
	}
	if len(s.logs) != 1 || s.logs[0].Message != "current log" || s.processed != 7 {
		t.Fatalf("narrowing range should retain cache and counters, got logs=%#v processed=%d", s.logs, s.processed)
	}
	var payload map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode range response: %v", err)
	}
	if payload["status"] != "filtered" {
		t.Fatalf("expected view-only filtering status, got %#v", payload)
	}
}

func TestLogRangeNarrowsWithPendingHistoryReloadsCleanly(t *testing.T) {
	now := time.Now().UnixMilli()
	existing := LogEntry{ID: 1, Timestamp: now, Message: "partial week log", Level: "info", Node: "node", Container: "api", nodeID: "node-1"}
	key := containerLogKey("node-1", "container-1")
	s := &server{
		nodes:                 []Node{{ID: "node-1", Name: "node"}},
		logs:                  []LogEntry{existing},
		processed:             7,
		historyRange:          "1w",
		historyPending:        1,
		rules:                 map[string]bool{},
		containerNames:        map[string]map[string]string{},
		containerLogs:         map[string][]LogEntry{key: {existing}},
		historyCoverage:       map[string]time.Time{key: time.Now().Add(-7 * 24 * time.Hour)},
		historyLoads:          map[string]struct{}{key: {}},
		historyLoadGeneration: map[string]uint64{key: 0},
		nodeContexts:          map[string]context.Context{},
		subscribers:           map[chan LogEntry]struct{}{},
		streams:               map[string]struct{}{},
		nodeCancels:           map[string]context.CancelFunc{},
	}

	response := httptest.NewRecorder()
	s.handleLogRange(response, httptest.NewRequest(http.MethodPost, "/api/logs/range?range=1d", nil))
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected accepted range response, got %d", response.Code)
	}
	if len(s.logs) != 0 || len(s.containerLogs) != 0 || len(s.historyCoverage) != 0 {
		t.Fatalf("narrowing an unfinished load should clear partial cache, got logs=%#v caches=%#v coverage=%#v", s.logs, s.containerLogs, s.historyCoverage)
	}
	var payload map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode range response: %v", err)
	}
	if payload["status"] != "reloading" {
		t.Fatalf("expected strict reload status, got %#v", payload)
	}
}

func TestFetchDozzleHistoryStopsWhenCacheIsFull(t *testing.T) {
	previousCapacity := maxStoredLogs
	maxStoredLogs = 1
	defer func() { maxStoredLogs = previousCapacity }()

	requests := 0
	dozzle := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		t.Fatal("history request should not start when the shared cache is full")
	}))
	defer dozzle.Close()

	s := &server{
		logs:            []LogEntry{{ID: 1, Message: "cached"}},
		historyCoverage: make(map[string]time.Time),
	}
	s.fetchDozzleHistory(context.Background(), Node{ID: "node-1", baseURL: dozzle.URL}, "host-1", "container-1", time.Now().Add(-time.Hour), time.Now(), 0)
	if requests != 0 {
		t.Fatalf("expected no history request at capacity, got %d", requests)
	}
}

func TestCacheObservabilityReportsEvictedLogs(t *testing.T) {
	previousCapacity := maxStoredLogs
	maxStoredLogs = 2
	defer func() { maxStoredLogs = previousCapacity }()

	now := time.Now()
	s := &server{
		nodes:                 []Node{{ID: "node-1", Name: "node"}},
		logs:                  []LogEntry{{ID: 2, Timestamp: now.Add(-time.Minute).UnixMilli(), Message: "newer"}, {ID: 1, Timestamp: now.Add(-2 * time.Minute).UnixMilli(), Message: "oldest"}},
		nextLogID:             2,
		containerLogs:         map[string][]LogEntry{},
		subscribers:           map[chan LogEntry]struct{}{},
		historyCoverage:       map[string]time.Time{},
		historyRange:          "30m",
		rules:                 map[string]bool{},
		nodeContexts:          map[string]context.Context{},
		nodeCancels:           map[string]context.CancelFunc{},
		containerNames:        map[string]map[string]string{},
		historyLoads:          map[string]struct{}{},
		historyLoadGeneration: map[string]uint64{},
	}

	s.appendRemoteLogLocked("node-1", dozzleLogEvent{ID: 3, Timestamp: now.UnixMilli(), Level: "info", Container: "container-1"}, "current", false)
	if s.evictedLogs != 1 || s.lastEvictedAt <= 0 {
		t.Fatalf("expected one observed eviction, got evicted=%d last=%d", s.evictedLogs, s.lastEvictedAt)
	}
	if len(s.logs) != 2 || s.logs[len(s.logs)-1].Message != "newer" {
		t.Fatalf("expected the oldest cached log to be evicted, got %#v", s.logs)
	}

	response := httptest.NewRecorder()
	s.handleBootstrap(response, httptest.NewRequest(http.MethodGet, "/api/bootstrap", nil))
	var payload bootstrapResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode bootstrap response: %v", err)
	}
	if payload.Storage.Used != 2 || payload.Storage.Capacity != 2 || payload.Storage.Evicted != 1 || payload.Storage.LastEvictedAt <= 0 {
		t.Fatalf("unexpected cache observability payload: %#v", payload.Storage)
	}
}

func TestFetchDozzleHistoryPaginatesFullPages(t *testing.T) {
	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour)
	pageOldest := now.Add(-1 * time.Hour)
	requests := 0
	dozzle := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/x-jsonl")
		if requests == 1 {
			for index := 0; index < dozzleHistoryPageSize; index++ {
				timestamp := pageOldest.Add(time.Duration(index) * time.Millisecond).UnixMilli()
				_, _ = fmt.Fprintf(w, `{"t":"single","m":"page one %d","rm":"page one %d","ts":%d,"id":%d,"l":"info","c":"container-1"}`+"\n", index, index, timestamp, index+1)
			}
			return
		}
		_, _ = fmt.Fprintf(w, `{"t":"single","m":"page two","rm":"page two","ts":%d,"id":1001,"l":"info","c":"container-1"}`+"\n", now.Add(-12*time.Hour).UnixMilli())
	}))
	defer dozzle.Close()

	s := &server{
		nodes:             []Node{{ID: "node-1", Name: "node", baseURL: dozzle.URL}},
		logs:              []LogEntry{},
		historyRange:      "1d",
		historyGeneration: 0,
		rules:             map[string]bool{},
		containerNames:    map[string]map[string]string{"node-1": {"container-1": "api"}},
		containerLogs:     map[string][]LogEntry{},
		subscribers:       map[chan LogEntry]struct{}{},
		streams:           map[string]struct{}{},
		nodeContexts:      map[string]context.Context{},
		nodeCancels:       map[string]context.CancelFunc{},
	}

	s.fetchDozzleHistory(context.Background(), s.nodes[0], "host-1", "container-1", from, now, 0)
	if requests != 2 {
		t.Fatalf("expected a second page request after a full page, got %d requests", requests)
	}
	if len(s.logs) != dozzleHistoryPageSize+1 {
		t.Fatalf("expected all paginated history to be cached, got %d logs", len(s.logs))
	}
}

func TestBootstrapSinceReturnsOnlyIncrementalLogs(t *testing.T) {
	s := &server{
		logs:        []LogEntry{{ID: 1, Timestamp: 1}, {ID: 2, Timestamp: 2}, {ID: 3, Timestamp: 3}},
		nodes:       []Node{},
		rules:       map[string]bool{},
		subscribers: map[chan LogEntry]struct{}{},
	}
	response := httptest.NewRecorder()
	s.handleBootstrap(response, httptest.NewRequest(http.MethodGet, "/api/bootstrap?since=1", nil))
	var payload bootstrapResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode bootstrap response: %v", err)
	}
	if len(payload.Logs) != 2 || payload.Logs[0].ID != 2 || payload.Logs[1].ID != 3 {
		t.Fatalf("expected only logs after the cursor, got %#v", payload.Logs)
	}
}

func (s *server) historyLoading() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.historyPending > 0
}

func TestClearLogCacheHandler(t *testing.T) {
	s := newServer()
	s.mu.Lock()
	s.logs = []LogEntry{{ID: 1, Message: "one"}, {ID: 2, Message: "two"}}
	s.containerLogs["node::container"] = []LogEntry{{ID: 1}}
	s.historyCoverage["node::container"] = time.Now()
	s.evictedLogs = 3
	s.lastEvictedAt = 123
	s.mu.Unlock()

	response := httptest.NewRecorder()
	s.handleClearLogCache(response, httptest.NewRequest(http.MethodPost, "/api/logs/cache/clear", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	var payload struct {
		Cleared int          `json:"cleared"`
		Storage storageStats `json:"storage"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode clear cache response: %v", err)
	}
	if payload.Cleared != 2 {
		t.Fatalf("expected 2 cleared logs, got %d", payload.Cleared)
	}
	if payload.Storage.Used != 0 || payload.Storage.Capacity != maxStoredLogs || payload.Storage.Evicted != 0 {
		t.Fatalf("unexpected storage stats after clear: %#v", payload.Storage)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.logs) != 0 || len(s.containerLogs) != 0 || len(s.historyCoverage) != 0 {
		t.Fatalf("expected caches to be empty, got logs=%d containerLogs=%d coverage=%d", len(s.logs), len(s.containerLogs), len(s.historyCoverage))
	}
	if s.evictedLogs != 0 || s.lastEvictedAt != 0 {
		t.Fatalf("expected eviction stats reset, got evicted=%d lastEvictedAt=%d", s.evictedLogs, s.lastEvictedAt)
	}

	methodResponse := httptest.NewRecorder()
	s.handleClearLogCache(methodResponse, httptest.NewRequest(http.MethodGet, "/api/logs/cache/clear", nil))
	if methodResponse.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405 for GET, got %d", methodResponse.Code)
	}
}

// Verifies the exact response shape the frontend consumes when the cache is
// non-empty, and that a second clear is a no-op reporting zero.
func TestUpdateNodeContainersPurgesHistoryOfRemovedContainers(t *testing.T) {
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	resetConfigDirCache(t)

	s := newServer()
	s.mu.Lock()
	s.nodes = []Node{{ID: "node-db-1", Name: "溯帆", URL: "http://127.0.0.1:1", hostID: "host-1", Status: "connected"}}
	s.containerNames["node-db-1"] = map[string]string{
		"70902cc1c45b": "dify-ssrf_proxy-1",
		"017213ca3c4b": "docker-api-1",
	}
	s.containerLogs["node-db-1::70902cc1c45b"] = []LogEntry{{ID: 1, Message: "from the container that was replaced"}}
	s.containerLogs["node-db-1::017213ca3c4b"] = []LogEntry{{ID: 2, Message: "still alive"}}
	s.historyCoverage["node-db-1::70902cc1c45b"] = time.Now()
	s.historyCoverage["node-db-1::017213ca3c4b"] = time.Now()
	s.logs = []LogEntry{{ID: 1, Message: "from the container that was replaced", nodeID: "node-db-1"}, {ID: 2, Message: "still alive", nodeID: "node-db-1"}}
	s.historyNodeScope = "nodes=node-db-1|containers="
	s.mu.Unlock()

	// The remote host replaced dify-ssrf_proxy-1 with docker-nginx-1. The stale
	// container is gone; docker-api-1 survived.
	s.updateNodeContainers(context.Background(), "node-db-1", []dozzleContainer{
		{ID: "017213ca3c4b", Name: "docker-api-1", State: "running"},
		{ID: "bb9708a43769", Name: "docker-nginx-1", State: "running"},
	})

	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, exists := s.containerLogs["node-db-1::70902cc1c45b"]; exists {
		t.Fatalf("history for the replaced container was not purged: %#v", s.containerLogs)
	}
	if _, exists := s.historyCoverage["node-db-1::70902cc1c45b"]; exists {
		t.Fatalf("history coverage for the replaced container was not purged")
	}
	if _, exists := s.containerLogs["node-db-1::017213ca3c4b"]; !exists {
		t.Fatalf("history for a surviving container must be kept: %#v", s.containerLogs)
	}
	if got := s.containerNames["node-db-1"]; len(got) != 2 || got["bb9708a43769"] != "docker-nginx-1" {
		t.Fatalf("container names were not replaced: %#v", got)
	}
	if s.historyNodeScope != "" {
		t.Fatalf("cached history scope must be invalidated so the window reloads, got %q", s.historyNodeScope)
	}
}

// TestUpdateNodeContainersConcurrentlyKeepsNodeOwnership guards the symptom
// seen in production: two nodes on the same subnet (晞飞科技 and 珈黛AI工坊)
// ended up holding each other's container lists, so every history request for
// one of them 404'd against the other's container IDs. Each node's map must
// only ever contain the set its own event stream delivered.
func TestUpdateNodeContainersConcurrentlyKeepsNodeOwnership(t *testing.T) {
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	resetConfigDirCache(t)

	s := newServer()
	s.mu.Lock()
	s.nodes = []Node{
		{ID: "node-db-5", Name: "晞飞科技", URL: "http://115.190.52.46:8099", hostID: "host-xf", Status: "connected"},
		{ID: "node-db-4", Name: "珈黛AI工坊", URL: "http://115.190.52.221:8099", hostID: "host-jd", Status: "connected"},
	}
	s.mu.Unlock()

	xf := []dozzleContainer{
		{ID: "762902c60121", Name: "nginx", State: "running", Host: "host-xf"},
		{ID: "21aa151d6807", Name: "dify-api-1", State: "running", Host: "host-xf"},
	}
	jd := []dozzleContainer{
		{ID: "d30ffba01831", Name: "dify-init_permissions-1", State: "running", Host: "host-jd"},
		{ID: "285f5a6b1da7", Name: "dify-weaviate-1", State: "running", Host: "host-jd"},
	}

	var wg sync.WaitGroup
	for round := 0; round < 8; round++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			s.updateNodeContainers(context.Background(), "node-db-5", xf)
		}()
		go func() {
			defer wg.Done()
			s.updateNodeContainers(context.Background(), "node-db-4", jd)
		}()
	}
	wg.Wait()

	s.mu.RLock()
	defer s.mu.RUnlock()
	gotXF := s.containerNames["node-db-5"]
	gotJD := s.containerNames["node-db-4"]
	if len(gotXF) != 2 || gotXF["762902c60121"] != "nginx" || gotXF["21aa151d6807"] != "dify-api-1" {
		t.Fatalf("晞飞科技 does not hold its own containers: %#v", gotXF)
	}
	if len(gotJD) != 2 || gotJD["d30ffba01831"] == "" || gotJD["285f5a6b1da7"] == "" {
		t.Fatalf("珈黛AI工坊 does not hold its own containers: %#v", gotJD)
	}
	if _, swapped := gotXF["285f5a6b1da7"]; swapped {
		t.Fatalf("晞飞科技 inherited 珈黛AI工坊's container: %#v", gotXF)
	}
	if _, swapped := gotJD["21aa151d6807"]; swapped {
		t.Fatalf("珈黛AI工坊 inherited 晞飞科技's container: %#v", gotJD)
	}
}

// TestUpdateNodeContainersRejectsForeignHostPayload covers the failure observed
// in production behind an HTTP proxy that delivered one node's answer to
// another node on the same subnet. The containers of 晞飞科技 were committed
// under 珈黛AI工坊 and vice versa, so every history fetch for those nodes 404'd
// and the dashboard showed no logs for them at all. A payload whose host tag
// disagrees with the node it arrived on must be refused rather than stored.
func TestUpdateNodeContainersRejectsForeignHostPayload(t *testing.T) {
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	resetConfigDirCache(t)

	s := newServer()
	s.mu.Lock()
	s.nodes = []Node{
		{ID: "node-db-4", Name: "珈黛AI工坊", URL: "http://115.190.52.221:8099", hostID: "host-jd", Status: "connected"},
	}
	s.containerNames["node-db-4"] = map[string]string{"018522750d15": "postgres"}
	s.mu.Unlock()

	// 晞飞科技's containers arrive on 珈黛AI工坊's request.
	s.updateNodeContainers(context.Background(), "node-db-4", []dozzleContainer{
		{ID: "762902c60121", Name: "nginx", State: "running", Host: "host-xf"},
		{ID: "21aa151d6807", Name: "dify-api-1", State: "running", Host: "host-xf"},
	})

	s.mu.RLock()
	defer s.mu.RUnlock()
	if got := s.containerNames["node-db-4"]; len(got) != 1 || got["018522750d15"] != "postgres" {
		t.Fatalf("a foreign host payload must not replace the node's containers: %#v", got)
	}
	for _, node := range s.nodes {
		if node.ID != "node-db-4" {
			continue
		}
		if node.Status != "error" {
			t.Fatalf("node status should surface the mismatch, got %q", node.Status)
		}
		if !strings.Contains(node.Error, "host-xf") || !strings.Contains(node.Error, "host-jd") {
			t.Fatalf("error should name both hosts, got %q", node.Error)
		}
	}
}

func TestUpdateNodeContainersKeepsHistoryWhenSetIsUnchanged(t *testing.T) {
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	resetConfigDirCache(t)

	s := newServer()
	s.mu.Lock()
	s.nodes = []Node{{ID: "node-db-1", Name: "溯帆", URL: "http://127.0.0.1:1", hostID: "host-1", Status: "connected"}}
	s.containerNames["node-db-1"] = map[string]string{"017213ca3c4b": "docker-api-1"}
	s.containerLogs["node-db-1::017213ca3c4b"] = []LogEntry{{ID: 1, Message: "keep me"}}
	s.historyCoverage["node-db-1::017213ca3c4b"] = time.Now()
	s.historyNodeScope = "nodes=node-db-1|containers="
	s.mu.Unlock()

	// Dozzle re-emits containers-changed on every connect and on state churn.
	// An identical set must not throw away history the user is reading.
	s.updateNodeContainers(context.Background(), "node-db-1", []dozzleContainer{
		{ID: "017213ca3c4b", Name: "docker-api-1", State: "running"},
	})

	s.mu.RLock()
	defer s.mu.RUnlock()
	if logs := s.containerLogs["node-db-1::017213ca3c4b"]; len(logs) != 1 {
		t.Fatalf("unchanged container set must not purge history, got %#v", logs)
	}
	if s.historyNodeScope != "nodes=node-db-1|containers=" {
		t.Fatalf("unchanged container set must not invalidate the history scope, got %q", s.historyNodeScope)
	}
}

func TestSameContainerSetIgnoresFirstEventAndDetectsRename(t *testing.T) {
	if !sameContainerSet(nil, map[string]string{"a": "a"}) {
		t.Fatal("the first containers-changed event has nothing to purge and must count as unchanged")
	}
	if sameContainerSet(map[string]string{"a": "old-name"}, map[string]string{"a": "new-name"}) {
		t.Fatal("a container renamed behind the same id must be treated as a change")
	}
	if !sameContainerSet(map[string]string{"a": "a", "b": "b"}, map[string]string{"b": "b", "a": "a"}) {
		t.Fatal("map iteration order must not affect the comparison")
	}
}

func TestNodeHistoryIsStaleBlocksRefillAfterPurge(t *testing.T) {
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	resetConfigDirCache(t)

	s := newServer()
	s.mu.Lock()
	s.nodes = []Node{{ID: "node-db-1", Name: "溯帆", URL: "http://127.0.0.1:1", hostID: "host-1", Status: "connected"}}
	s.containerNames["node-db-1"] = map[string]string{"70902cc1c45b": "dify-ssrf_proxy-1"}
	s.containerLogs["node-db-1::70902cc1c45b"] = []LogEntry{{ID: 1, Message: "stale"}}
	s.mu.Unlock()

	requests := 0
	dozzle := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		t.Errorf("a fetch for a purged node must not reach Dozzle: %s", r.URL.String())
	}))
	defer dozzle.Close()

	// A purge marks the node stale until the fresh container list is committed.
	s.purgeNodeHistory(context.Background(), "node-db-1", map[string]string{"bb9708a43769": "docker-nginx-1"})
	if !s.nodeHistoryIsStale("node-db-1") {
		t.Fatal("purge must mark the node's history stale")
	}

	s.fetchDozzleHistory(context.Background(), Node{ID: "node-db-1", Name: "溯帆", URL: dozzle.URL, baseURL: dozzle.URL}, "host-1", "70902cc1c45b", time.Now().Add(-time.Hour), time.Now(), s.historyGeneration)
	if requests != 0 {
		t.Fatalf("expected no request for a stale node, got %d", requests)
	}
	if _, exists := s.containerLogs["node-db-1::70902cc1c45b"]; exists {
		t.Fatalf("a stale fetch must not re-populate purged history: %#v", s.containerLogs)
	}
}

func TestClearLogCacheReportsCountAndResetsStats(t *testing.T) {
	s := newServer()
	s.mu.Lock()
	for i := 1; i <= 25; i++ {
		s.logs = append(s.logs, LogEntry{ID: int64(i), Message: "seed"})
	}
	s.containerLogs["node::api"] = s.logs
	s.historyCoverage["node::api"] = time.Now()
	s.evictedLogs = 7
	s.lastEvictedAt = 99999
	s.processed = 25
	s.mu.Unlock()

	decode := func(rec *httptest.ResponseRecorder) (int, storageStats) {
		t.Helper()
		var payload struct {
			Cleared int          `json:"cleared"`
			Storage storageStats `json:"storage"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return payload.Cleared, payload.Storage
	}

	first := httptest.NewRecorder()
	s.handleClearLogCache(first, httptest.NewRequest(http.MethodPost, "/api/logs/cache/clear", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first clear status = %d, want 200", first.Code)
	}
	cleared, storage := decode(first)
	if cleared != 25 {
		t.Fatalf("cleared = %d, want 25", cleared)
	}
	if storage.Used != 0 || storage.Percent != 0 || storage.Evicted != 0 || storage.LastEvictedAt != 0 {
		t.Fatalf("storage not reset: %#v", storage)
	}

	s.mu.RLock()
	keptProcessed := s.processed
	s.mu.RUnlock()
	if keptProcessed != 25 {
		t.Fatalf("processed counter should be preserved, got %d", keptProcessed)
	}

	second := httptest.NewRecorder()
	s.handleClearLogCache(second, httptest.NewRequest(http.MethodPost, "/api/logs/cache/clear", nil))
	clearedAgain, _ := decode(second)
	if clearedAgain != 0 {
		t.Fatalf("second clear reported %d, want 0", clearedAgain)
	}
}

// --- storage backend selection -------------------------------------------

func TestBackendForRequestDistinguishesLocalFromRemote(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		want       configBackend
	}{
		{name: "ipv4 loopback", remoteAddr: "127.0.0.1:51234", want: backendFile},
		{name: "ipv6 loopback", remoteAddr: "[::1]:51234", want: backendFile},
		{name: "ipv6 mapped ipv4 loopback", remoteAddr: "[::ffff:127.0.0.1]:51234", want: backendFile},
		{name: "loopback without port", remoteAddr: "127.0.0.1", want: backendFile},
		{name: "lan address", remoteAddr: "192.168.4.84:51234", want: backendBrowser},
		{name: "public address", remoteAddr: "124.174.71.198:51234", want: backendBrowser},
		{name: "ipv6 mapped lan address", remoteAddr: "[::ffff:192.168.4.84]:51234", want: backendBrowser},
		{name: "garbage", remoteAddr: "not-an-address", want: backendBrowser},
		{name: "empty", remoteAddr: "", want: backendBrowser},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/config/info", nil)
			request.RemoteAddr = testCase.remoteAddr
			if got := backendForRequest(request); got != testCase.want {
				t.Fatalf("backendForRequest(%q) = %s, want %s", testCase.remoteAddr, got, testCase.want)
			}
		})
	}
}

// A remote caller must never be able to write to the owner's configuration.
func TestNodeWritesRejectedForRemoteCaller(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("LOG_AGENT_CONFIG_DIR", configRoot)
	resetConfigDirCache(t)

	s := newServer()
	body := bytes.NewBufferString(`{"name":"injected","url":"http://127.0.0.1:9999"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/nodes", body)
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "124.174.71.198:51234"
	response := httptest.NewRecorder()

	s.handleNodes(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("remote node create returned %d, want 403", response.Code)
	}
	if _, err := os.Stat(nodesFilePath()); err == nil {
		t.Fatal("a remote request created the node configuration file")
	}
}

func TestNodeWriteAllowedForLocalCaller(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("LOG_AGENT_CONFIG_DIR", configRoot)
	resetConfigDirCache(t)

	s := newServer()
	body := bytes.NewBufferString(`{"name":"local","url":"http://127.0.0.1:9999"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/nodes", body)
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "127.0.0.1:51234"
	response := httptest.NewRecorder()

	s.handleNodes(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("local node create returned %d, want 201: %s", response.Code, response.Body.String())
	}
	nodes, err := s.store.listNodes()
	if err != nil {
		t.Fatalf("list stored nodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected the node to be persisted, got %d", len(nodes))
	}
}

// The settings panel exposes exactly one storage affordance: the reveal link in
// its header. A visitor to someone else's deployment must never be offered it,
// because the configuration files are not on their machine.
//
// The response deliberately carries no filesystem paths at all, so there is
// nothing left to leak here; what this pins down is that the two booleans that
// gate the link stay false for a remote caller.
func TestConfigInfoHidesFileAccessFromRemoteCaller(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("LOG_AGENT_CONFIG_DIR", configRoot)
	resetConfigDirCache(t)

	s := newServer()
	request := httptest.NewRequest(http.MethodGet, "/api/config/info", nil)
	request.RemoteAddr = "8.8.8.8:51234"
	response := httptest.NewRecorder()
	s.handleConfigInfo(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("config info returned %d, want 200", response.Code)
	}
	var remote configInfoResponse
	if err := json.Unmarshal(response.Body.Bytes(), &remote); err != nil {
		t.Fatalf("decode remote config info: %v", err)
	}
	if remote.LocalMode {
		t.Fatal("remote caller was reported as local")
	}
	if remote.CanReveal {
		t.Fatal("remote caller was offered the file-manager link")
	}

	// A loopback caller is the owner of the files, so the link is allowed. Only
	// localMode is asserted unconditionally: canReveal additionally depends on
	// the platform having a desktop, which a headless CI box does not.
	localRequest := httptest.NewRequest(http.MethodGet, "/api/config/info", nil)
	localRequest.RemoteAddr = "127.0.0.1:51234"
	localResponse := httptest.NewRecorder()
	s.handleConfigInfo(localResponse, localRequest)
	var local configInfoResponse
	if err := json.Unmarshal(localResponse.Body.Bytes(), &local); err != nil {
		t.Fatalf("decode local config info: %v", err)
	}
	if !local.LocalMode {
		t.Fatal("loopback caller was not reported as local")
	}
	if local.CanReveal != hasDesktopSession() {
		t.Fatalf("local canReveal = %t, want %t (desktop session)", local.CanReveal, hasDesktopSession())
	}
}

// TestAIProfileWriteNeedsNoTokenWhenNoneConfigured guards a first-run deadlock.
//
// Saving a provider used to demand an admin token unconditionally, while every
// other privileged endpoint treats "no token configured" as "nothing to check".
// With no token set the two disagree: the client is prompted for a value, but
// authorizedAdminRequest compares it against an empty expected value and can
// never match, so the save is rejected no matter what is typed and the prompt
// reappears forever. Setup could not be completed through the UI at all.
func TestAIProfileWriteNeedsNoTokenWhenNoneConfigured(t *testing.T) {
	configRoot := t.TempDir()
	t.Setenv("LOG_AGENT_CONFIG_DIR", configRoot)
	resetConfigDirCache(t)

	s := newServer()
	body := `{"name":"供应商","baseURL":"https://example.com","apiKey":"sk-x","type":"anthropic","models":[{"name":"m"}]}`

	// No token configured: the write must be accepted.
	create := httptest.NewRequest(http.MethodPost, "/api/ai/profiles", strings.NewReader(body))
	create.RemoteAddr = "127.0.0.1:51234"
	createResponse := httptest.NewRecorder()
	s.handleAIProfiles(createResponse, create)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("save without a configured token returned %d (%s), want 200",
			createResponse.Code, createResponse.Body.String())
	}

	// Once a token exists, the same request must be rejected unless it matches.
	s.mu.Lock()
	s.settings.AdminToken = "secret"
	s.mu.Unlock()

	rejected := httptest.NewRequest(http.MethodPost, "/api/ai/profiles", strings.NewReader(body))
	rejected.RemoteAddr = "127.0.0.1:51234"
	rejectedResponse := httptest.NewRecorder()
	s.handleAIProfiles(rejectedResponse, rejected)
	if rejectedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("save without the token returned %d, want 401", rejectedResponse.Code)
	}

	accepted := httptest.NewRequest(http.MethodPost, "/api/ai/profiles", strings.NewReader(body))
	accepted.RemoteAddr = "127.0.0.1:51234"
	accepted.Header.Set("X-Log-Agent-Admin-Token", "secret")
	acceptedResponse := httptest.NewRecorder()
	s.handleAIProfiles(acceptedResponse, accepted)
	if acceptedResponse.Code != http.StatusOK {
		t.Fatalf("save with the right token returned %d (%s), want 200",
			acceptedResponse.Code, acceptedResponse.Body.String())
	}
}

// The bootstrap tells the client which store to use, and it must follow the
// same rule as the write guards.
func TestBootstrapReportsStorageMode(t *testing.T) {
	for _, testCase := range []struct {
		remoteAddr string
		want       string
	}{
		{remoteAddr: "127.0.0.1:51234", want: string(backendFile)},
		{remoteAddr: "124.174.71.198:51234", want: string(backendBrowser)},
	} {
		s := &server{
			logs: nil, rules: map[string]bool{}, ruleOrder: defaultRuleOrder(),
			containerLogs: map[string][]LogEntry{}, historyCoverage: map[string]time.Time{},
			nodeContexts: map[string]context.Context{}, nodeCancels: map[string]context.CancelFunc{},
		}
		request := httptest.NewRequest(http.MethodGet, "/api/bootstrap", nil)
		request.RemoteAddr = testCase.remoteAddr
		response := httptest.NewRecorder()
		s.handleBootstrap(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("bootstrap returned %d, want 200", response.Code)
		}
		var payload bootstrapResponse
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode bootstrap: %v", err)
		}
		if payload.StorageMode != testCase.want {
			t.Fatalf("bootstrap storageMode for %s = %q, want %q", testCase.remoteAddr, payload.StorageMode, testCase.want)
		}
	}
}
