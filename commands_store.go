package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Command workspace storage.
//
// The server-commands page (flows, flow groups, the server library and the
// favourites) used to live only in the browser's localStorage, so it vanished
// with the browser profile and could not be backed up or copied with the exe.
// It is now kept in commands.json next to nodes.json and models.json.
//
// The backend does not interpret the flows: the page owns their shape and
// sends each section as a JSON array. The server only checks that every
// section is an array and writes the file atomically.

const (
	commandsFileName    = "commands.json"
	commandsMaxBodySize = 16 << 20
)

func commandsFilePath() string { return filepath.Join(configDir(), commandsFileName) }

// storedCommands is the on-disk shape of commands.json.
type storedCommands struct {
	Version   int             `json:"version"`
	UpdatedAt string          `json:"updated_at,omitempty"`
	Flows     json.RawMessage `json:"flows"`
	Groups    json.RawMessage `json:"groups"`
	Servers   json.RawMessage `json:"servers"`
	Favorites json.RawMessage `json:"favorites"`
}

var commandsFileMu sync.Mutex

// jsonArrayOrEmpty returns raw when it holds a JSON array, "[]" when it is
// absent or null, and false for anything else.
func jsonArrayOrEmpty(raw json.RawMessage) (json.RawMessage, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return json.RawMessage("[]"), true
	}
	var items []json.RawMessage
	if trimmed[0] != '[' || json.Unmarshal(trimmed, &items) != nil {
		return nil, false
	}
	return trimmed, true
}

func normalizeStoredCommands(value storedCommands) (storedCommands, bool) {
	sections := []*json.RawMessage{&value.Flows, &value.Groups, &value.Servers, &value.Favorites}
	for _, section := range sections {
		normalized, ok := jsonArrayOrEmpty(*section)
		if !ok {
			return value, false
		}
		*section = normalized
	}
	value.Version = 1
	return value, true
}

// handleCommandsState serves GET (read commands.json) and PUT (replace it).
// A GET before anything was saved answers {"exists": false} so the page can
// migrate what it still has in localStorage.
//
// Like nodes.json and models.json, the file belongs to the machine's own user:
// a visitor from elsewhere (backendBrowser) is told to keep using their
// browser storage, and may not write the server's disk.
func (s *server) handleCommandsState(w http.ResponseWriter, r *http.Request) {
	if backendForRequest(r) != backendFile {
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, map[string]any{"exists": false, "storage": string(backendBrowser)})
			return
		}
		writeAPIError(w, http.StatusForbidden, &apiError{code: "commands_local_only"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		commandsFileMu.Lock()
		var stored storedCommands
		err := readJSONFile(commandsFilePath(), &stored)
		commandsFileMu.Unlock()
		if errors.Is(err, errNoConfigFile) {
			writeJSON(w, http.StatusOK, map[string]any{"exists": false})
			return
		}
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, &apiError{code: "commands_read_failed", detail: err.Error()})
			return
		}
		normalized, ok := normalizeStoredCommands(stored)
		if !ok {
			writeAPIError(w, http.StatusInternalServerError, &apiError{code: "commands_read_failed", detail: "commands.json sections must be arrays"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"exists": true, "commands": normalized})
	case http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, commandsMaxBodySize)
		var request storedCommands
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeAPIError(w, http.StatusBadRequest, &apiError{code: "commands_invalid", detail: err.Error()})
			return
		}
		normalized, ok := normalizeStoredCommands(request)
		if !ok {
			writeAPIError(w, http.StatusBadRequest, &apiError{code: "commands_invalid", detail: "flows, groups, servers and favorites must be arrays"})
			return
		}
		normalized.UpdatedAt = time.Now().Format(time.RFC3339)
		commandsFileMu.Lock()
		defer commandsFileMu.Unlock()
		if err := os.MkdirAll(configDir(), 0o700); err != nil {
			writeAPIError(w, http.StatusInternalServerError, &apiError{code: "commands_write_failed", detail: err.Error()})
			return
		}
		if err := writeJSONFile(commandsFilePath(), normalized); err != nil {
			writeAPIError(w, http.StatusInternalServerError, &apiError{code: "commands_write_failed", detail: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "updated_at": normalized.UpdatedAt})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}
