package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// File-backed configuration store.
//
// Nodes and AI providers are small, low-write datasets (a handful of rows,
// edited by hand every few weeks), so they live in two JSON files inside the
// per-user config directory. That keeps a fresh install working with no
// database to provision and no server-side account to create.
//
// Every write goes through writeFileAtomic so a crash or a full disk cannot
// leave a half-written file behind, and an in-process mutex serialises updates
// so two concurrent requests cannot lose each other's changes.

const (
	nodesFileName  = "nodes.json"
	modelsFileName = "models.json"
)

// storedNode is the on-disk shape of a Dozzle node.
type storedNode struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Address    string `json:"address"`
	Style      string `json:"style"`
	CreateTime string `json:"create_time,omitempty"`
}

// storedModel is the on-disk shape of an AI provider. Type 1 is Anthropic,
// anything else is treated as an OpenAI-compatible endpoint.
type storedModel struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key,omitempty"`
	Type       int    `json:"type"`
	ModelName  string `json:"model_name"`
	CreateTime string `json:"create_time,omitempty"`
}

// configStore owns the two JSON files.
type configStore struct {
	mu      sync.Mutex
	dir     string
	nodes   []storedNode
	models  []storedModel
	loaded  bool
	loadErr error
}

var errNoConfigFile = errors.New("configuration file does not exist")

// configDirState memoises the resolved directory. The resolution includes a
// filesystem write probe, so it must not run on every request; and a running
// process must not silently start writing somewhere new halfway through.
type configDirState struct {
	once sync.Once
	dir  string
}

var configDirMemo configDirState

// configDirCache exposes the memo so tests can reset it. Production code only
// ever calls configDir().
func configDirCache() *configDirState { return &configDirMemo }

// configDir resolves the directory that holds nodes.json and models.json.
//
// Resolution order:
//  1. LOG_AGENT_CONFIG_DIR, for an explicit override.
//  2. The "data" folder beside the executable, when that is writable. This is
//     the normal case and keeps the install self-contained.
//  3. The per-user config directory, for binaries shipped in a read-only
//     location.
//
// A path relative to the working directory is never used: it would resolve
// differently depending on where the process was launched from, so
// double-clicking the executable and starting it from a shell would silently
// read two different files.
//
// "go run" is refused outright rather than accommodated. It compiles the
// program into the system temp directory and runs it from there, so rule 2
// resolves to a throwaway path that the Go toolchain deletes on exit: a user
// who configures nodes through `go run .` loses them the moment they stop the
// process, and the next `go run .` lands in a different hashed directory and
// appears empty. Silently writing somewhere disposable is worse than not
// starting, so validateConfigDir reports it and main() exits.
func configDir() string {
	configDirMemo.once.Do(func() {
		if custom := strings.TrimSpace(os.Getenv("LOG_AGENT_CONFIG_DIR")); custom != "" {
			configDirMemo.dir = custom
			return
		}
		if dir, ok := portableConfigDir(); ok {
			configDirMemo.dir = dir
			return
		}
		configDirMemo.dir = fallbackConfigDir()
	})
	return configDirMemo.dir
}

// withinDir reports whether path is dir itself or lives underneath it. Both
// are compared after resolution so a symlinked temp directory still matches.
func withinDir(path, dir string) bool {
	resolvedPath, err := filepath.Abs(path)
	if err != nil {
		resolvedPath = filepath.Clean(path)
	}
	resolvedDir, err := filepath.Abs(dir)
	if err != nil {
		resolvedDir = filepath.Clean(dir)
	}
	if samePath(resolvedPath, resolvedDir) {
		return true
	}
	return strings.HasPrefix(
		strings.ToLower(resolvedPath)+string(filepath.Separator),
		strings.ToLower(resolvedDir)+string(filepath.Separator),
	)
}

// samePath compares two absolute paths for equality, tolerating Windows' case
// insensitivity and a trailing separator.
func samePath(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(a, `\/`), strings.TrimRight(b, `\/`))
}

// validateConfigDir rejects a configuration directory that would not survive
// the process that created it.
//
// The case handled here is `go run`, which is both the most likely to happen to
// a developer and the one whose failure mode is silent data loss.
//
// The check is deliberately narrow: it looks for Go's build directory naming
// (go-build<digits>), not merely "somewhere under the temp directory". Anything
// that runs from temp would otherwise be refused, including the test binary
// that `go test` builds, which would make this guard unreachable by its own
// tests.
func validateConfigDir() error {
	if strings.TrimSpace(os.Getenv("LOG_AGENT_CONFIG_DIR")) != "" {
		// An explicit choice is always honoured, even inside a temp directory:
		// that is the documented escape hatch for running under `go run`.
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return nil
	}
	if !withinDir(executable, os.TempDir()) || !isGoRunExecutable(executable) {
		return nil
	}
	return fmt.Errorf(`检测到程序运行在 Go 的临时构建目录中，配置会在进程退出时被系统清理：
  可执行文件：%s

这通常是因为使用了 "go run ."。请改用以下任一方式启动：
  1. go build -o dozzle-ops.exe .  然后运行生成的 exe（配置存在 exe 旁边的 data/）
  2. 指定一个固定目录：LOG_AGENT_CONFIG_DIR=<目录> go run .`, executable)
}

// goBuildDirPattern matches the directory Go compiles into for `go run`: a
// "go-build" prefix followed by digits, e.g. go-build1234567890.
var goBuildDirPattern = regexp.MustCompile(`^go-build[0-9]+$`)

// isGoRunExecutable reports whether this process was started by `go run`.
//
// Path alone is not enough to tell them apart: `go test` builds into the same
// go-build<digits> tree, and refusing to run there would make the guard
// untestable and would block the test suite itself. The two are distinguishable
// by the artifact name, since the test toolchain appends ".test" to the binary
// it builds (`go test` yields dozzle-ops.test.exe, `go run` yields
// dozzle-ops.exe).
func isGoRunExecutable(path string) bool {
	if strings.HasSuffix(strings.ToLower(filepath.Base(path)), ".test.exe") {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if goBuildDirPattern.MatchString(part) {
			return true
		}
	}
	return false
}

// portableConfigDir returns the "data" folder next to the executable, which is
// where a single-file install keeps its configuration.
//
// The layout is deliberately flat and visible:
//
//	dozzle-ops.exe
//	data/nodes.json
//	data/models.json
//
// so the whole folder can be copied to another machine as-is. It is resolved
// from the executable path rather than the working directory, so
// double-clicking the binary and launching it from a shell read the same file.
//
// The second return value reports whether the location is usable: a program
// shipped inside a read-only directory such as /usr/bin or Program Files
// cannot write beside itself, and must not be handed a path that would fail on
// the first save.
func portableConfigDir() (string, bool) {
	executable, err := os.Executable()
	if err != nil {
		return "", false
	}
	dir := filepath.Join(filepath.Dir(executable), "data")
	if !isWritableDir(dir) {
		return "", false
	}
	return dir, true
}

// fallbackConfigDir is used when the executable's directory is not writable.
// It resolves through os.UserConfigDir so the location is still stable and
// independent of the working directory.
func fallbackConfigDir() string {
	if base, err := os.UserConfigDir(); err == nil && strings.TrimSpace(base) != "" {
		return filepath.Join(base, "logAgent")
	}
	return filepath.Join(os.TempDir(), "logAgent")
}

// isWritableDir reports whether dir exists (or can be created) and accepts a
// file. It probes with a real file instead of trusting the permission bits,
// because on Windows those bits do not reflect the effective ACL.
func isWritableDir(dir string) bool {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false
	}
	probe, err := os.CreateTemp(dir, ".write-probe-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return true
}

// nodesFilePath and modelsFilePath are exposed so the settings API can report
// the real location to the UI without duplicating the naming rules.
func nodesFilePath() string  { return filepath.Join(configDir(), nodesFileName) }
func modelsFilePath() string { return filepath.Join(configDir(), modelsFileName) }

func newConfigStore() *configStore {
	return &configStore{dir: configDir()}
}

// ensureDir creates the config directory on first write. Reads tolerate a
// missing directory and simply report "not configured yet".
func (c *configStore) ensureDir() error {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	return nil
}

// readJSONFile decodes path into target. A missing file is reported as
// errNoConfigFile so callers can treat "first run" differently from a real
// read failure such as a permissions problem or malformed JSON.
func readJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return errNoConfigFile
		}
		return fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		// An empty file is what a failed write used to leave behind. Treat it
		// as "no configuration" instead of a parse error so the service still
		// starts and the user can re-add their nodes.
		return errNoConfigFile
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	return nil
}

// writeFileAtomic writes data to path via a temporary file in the same
// directory followed by a rename, which is atomic on every supported platform.
// Writing in place would risk truncating the file if the process dies mid-write.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tempName := temp.Name()
	// Best-effort cleanup: once the rename succeeds these become no-ops.
	defer func() { _ = os.Remove(tempName) }()

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("flush temporary file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	// Secrets live in models.json, so keep the file owner-only on platforms
	// that honour the permission bits. This is a no-op on Windows, where
	// access is governed by the user profile ACL instead.
	if err := os.Chmod(tempName, 0o600); err != nil {
		return fmt.Errorf("set file permissions: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("replace %s: %w", filepath.Base(path), err)
	}
	return nil
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	// Trailing newline keeps the files friendly to manual editing and diffing,
	// which matters because the settings page invites users to edit them.
	return writeFileAtomic(path, append(data, '\n'))
}

// load reads both files. A missing file is not an error: it just means the
// user has not configured that domain yet.
func (c *configStore) load() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loadLocked()
}

func (c *configStore) loadLocked() error {
	c.nodes = nil
	c.models = nil

	var nodes []storedNode
	switch err := readJSONFile(nodesFilePath(), &nodes); {
	case err == nil:
		c.nodes = sanitizeStoredNodes(nodes)
	case errors.Is(err, errNoConfigFile):
	default:
		c.loadErr = err
	}

	var models []storedModel
	switch err := readJSONFile(modelsFilePath(), &models); {
	case err == nil:
		c.models = sanitizeStoredModels(models)
	case errors.Is(err, errNoConfigFile):
	default:
		if c.loadErr == nil {
			c.loadErr = err
		}
	}
	c.loaded = true
	return c.loadErr
}

// sanitizeStoredNodes drops entries that cannot produce a usable node and
// reassigns missing/duplicate IDs, so a hand-edited file cannot wedge startup.
func sanitizeStoredNodes(nodes []storedNode) []storedNode {
	cleaned := make([]storedNode, 0, len(nodes))
	seen := make(map[int64]struct{}, len(nodes))
	var nextID int64 = 1
	for _, node := range nodes {
		node.Name = strings.TrimSpace(node.Name)
		node.Address = strings.TrimSpace(node.Address)
		if node.Name == "" || node.Address == "" {
			continue
		}
		node.Style = normalizeNodeStyle(node.Style)
		if node.ID <= 0 {
			node.ID = nextID
		}
		for {
			if _, exists := seen[node.ID]; !exists {
				break
			}
			node.ID++
		}
		seen[node.ID] = struct{}{}
		if node.ID >= nextID {
			nextID = node.ID + 1
		}
		cleaned = append(cleaned, node)
	}
	return cleaned
}

// sanitizeStoredModels mirrors sanitizeStoredNodes for AI providers. Entries
// without a usable endpoint or model list are discarded rather than surfacing
// as broken entries in the model picker.
func sanitizeStoredModels(models []storedModel) []storedModel {
	cleaned := make([]storedModel, 0, len(models))
	seen := make(map[int64]struct{}, len(models))
	var nextID int64 = 1
	for _, model := range models {
		model.Name = strings.TrimSpace(model.Name)
		model.BaseURL = strings.TrimRight(strings.TrimSpace(model.BaseURL), "/")
		model.ModelName = strings.Trim(strings.TrimSpace(model.ModelName), ",")
		if model.Name == "" || model.BaseURL == "" || model.ModelName == "" {
			continue
		}
		if model.Type != 1 {
			model.Type = 2
		}
		if model.ID <= 0 {
			model.ID = nextID
		}
		for {
			if _, exists := seen[model.ID]; !exists {
				break
			}
			model.ID++
		}
		seen[model.ID] = struct{}{}
		if model.ID >= nextID {
			nextID = model.ID + 1
		}
		cleaned = append(cleaned, model)
	}
	return cleaned
}

func (c *configStore) listNodes() ([]storedNode, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.loaded {
		if err := c.loadLocked(); err != nil {
			return nil, err
		}
	}
	return append([]storedNode{}, c.nodes...), nil
}

func (c *configStore) listModels() ([]storedModel, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.loaded {
		if err := c.loadLocked(); err != nil {
			return nil, err
		}
	}
	return append([]storedModel{}, c.models...), nil
}

func (c *configStore) nextNodeIDLocked() int64 {
	var maxID int64
	for _, node := range c.nodes {
		if node.ID > maxID {
			maxID = node.ID
		}
	}
	return maxID + 1
}

func (c *configStore) nextModelIDLocked() int64 {
	var maxID int64
	for _, model := range c.models {
		if model.ID > maxID {
			maxID = model.ID
		}
	}
	return maxID + 1
}

// saveNodesLocked persists the current in-memory node list. Callers hold mu.
func (c *configStore) saveNodesLocked() error {
	if err := c.ensureDir(); err != nil {
		return err
	}
	sorted := append([]storedNode{}, c.nodes...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	return writeJSONFile(nodesFilePath(), sorted)
}

func (c *configStore) saveModelsLocked() error {
	if err := c.ensureDir(); err != nil {
		return err
	}
	sorted := append([]storedModel{}, c.models...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	return writeJSONFile(modelsFilePath(), sorted)
}

// insertNode appends a node and persists the file, returning the assigned ID.
func (c *configStore) insertNode(name, address, style string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadLockedIfNeeded(); err != nil {
		return 0, err
	}
	record := storedNode{
		ID:         c.nextNodeIDLocked(),
		Name:       name,
		Address:    address,
		Style:      normalizeNodeStyle(style),
		CreateTime: time.Now().Format(time.RFC3339),
	}
	previous := c.nodes
	c.nodes = append(append([]storedNode{}, c.nodes...), record)
	if err := c.saveNodesLocked(); err != nil {
		c.nodes = previous
		return 0, err
	}
	return record.ID, nil
}

func (c *configStore) updateNode(id int64, name, address, style string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadLockedIfNeeded(); err != nil {
		return err
	}
	index := -1
	for i, node := range c.nodes {
		if node.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return errRecordNotFound
	}
	previous := c.nodes
	updated := append([]storedNode{}, c.nodes...)
	updated[index].Name = name
	updated[index].Address = address
	updated[index].Style = normalizeNodeStyle(style)
	c.nodes = updated
	if err := c.saveNodesLocked(); err != nil {
		c.nodes = previous
		return err
	}
	return nil
}

func (c *configStore) deleteNode(id int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadLockedIfNeeded(); err != nil {
		return err
	}
	index := -1
	for i, node := range c.nodes {
		if node.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return errRecordNotFound
	}
	previous := c.nodes
	updated := append([]storedNode{}, c.nodes[:index]...)
	updated = append(updated, c.nodes[index+1:]...)
	c.nodes = updated
	if err := c.saveNodesLocked(); err != nil {
		c.nodes = previous
		return err
	}
	return nil
}

func (c *configStore) loadLockedIfNeeded() error {
	if c.loaded {
		return nil
	}
	return c.loadLocked()
}

var errRecordNotFound = errors.New("record not found")

// upsertModel creates or updates an AI provider. An empty apiKey on an update
// preserves the stored key, matching the behaviour users expect from a form
// that never echoes secrets back to the browser.
func (c *configStore) upsertModel(id int64, name, baseURL, apiKey string, profileType int, modelNames []string) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadLockedIfNeeded(); err != nil {
		return 0, err
	}
	joined := strings.Join(modelNames, ",")
	if profileType != 1 {
		profileType = 2
	}

	if id > 0 {
		index := -1
		for i, model := range c.models {
			if model.ID == id {
				index = i
				break
			}
		}
		if index < 0 {
			return 0, errRecordNotFound
		}
		previous := c.models
		updated := append([]storedModel{}, c.models...)
		updated[index].Name = name
		updated[index].BaseURL = baseURL
		updated[index].Type = profileType
		updated[index].ModelName = joined
		if strings.TrimSpace(apiKey) != "" {
			updated[index].APIKey = strings.TrimSpace(apiKey)
		}
		c.models = updated
		if err := c.saveModelsLocked(); err != nil {
			c.models = previous
			return 0, err
		}
		return id, nil
	}

	record := storedModel{
		ID:         c.nextModelIDLocked(),
		Name:       name,
		BaseURL:    baseURL,
		APIKey:     strings.TrimSpace(apiKey),
		Type:       profileType,
		ModelName:  joined,
		CreateTime: time.Now().Format(time.RFC3339),
	}
	previous := c.models
	c.models = append(append([]storedModel{}, c.models...), record)
	if err := c.saveModelsLocked(); err != nil {
		c.models = previous
		return 0, err
	}
	return record.ID, nil
}

func (c *configStore) deleteModel(id int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadLockedIfNeeded(); err != nil {
		return err
	}
	index := -1
	for i, model := range c.models {
		if model.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return errRecordNotFound
	}
	previous := c.models
	updated := append([]storedModel{}, c.models[:index]...)
	updated = append(updated, c.models[index+1:]...)
	c.models = updated
	if err := c.saveModelsLocked(); err != nil {
		c.models = previous
		return err
	}
	return nil
}

// findModel returns one stored provider by ID.
func (c *configStore) findModel(id int64) (storedModel, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadLockedIfNeeded(); err != nil {
		return storedModel{}, err
	}
	for _, model := range c.models {
		if model.ID == id {
			return model, nil
		}
	}
	return storedModel{}, errRecordNotFound
}

// replaceAll swaps in a full configuration, used by the import endpoint.
// The files are written before the in-memory state is published so a failed
// import leaves the running service untouched.
func (c *configStore) replaceAll(nodes []storedNode, models []storedModel) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	cleanedNodes := sanitizeStoredNodes(nodes)
	cleanedModels := sanitizeStoredModels(models)

	if err := c.ensureDir(); err != nil {
		return err
	}
	sortedNodes := append([]storedNode{}, cleanedNodes...)
	sort.SliceStable(sortedNodes, func(i, j int) bool { return sortedNodes[i].ID < sortedNodes[j].ID })
	if err := writeJSONFile(nodesFilePath(), sortedNodes); err != nil {
		return err
	}
	sortedModels := append([]storedModel{}, cleanedModels...)
	sort.SliceStable(sortedModels, func(i, j int) bool { return sortedModels[i].ID < sortedModels[j].ID })
	if err := writeJSONFile(modelsFilePath(), sortedModels); err != nil {
		return err
	}

	c.nodes = sortedNodes
	c.models = sortedModels
	c.loaded = true
	c.loadErr = nil
	return nil
}
