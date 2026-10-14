package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// Command run logs.
//
// Every run of a flow records what each line sent and what the server
// answered. The page builds the records and keeps them, newest first, in
// data/command-logs.json under the same rule as commands.json: only callers on
// the machine running the service use the file, visitors keep theirs in their
// own browser. The backend stores the array as-is and caps its length.

const (
	commandLogsFileName    = "command-logs.json"
	commandLogsMaxBodySize = 32 << 20
	commandLogsMaxRuns     = 200
)

func commandLogsFilePath() string { return filepath.Join(configDir(), commandLogsFileName) }

var commandLogsFileMu sync.Mutex

func (s *server) handleCommandLogs(w http.ResponseWriter, r *http.Request) {
	if backendForRequest(r) != backendFile {
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, map[string]any{"storage": string(backendBrowser), "runs": []any{}})
			return
		}
		writeAPIError(w, http.StatusForbidden, &apiError{code: "commands_local_only"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		commandLogsFileMu.Lock()
		var runs []json.RawMessage
		err := readJSONFile(commandLogsFilePath(), &runs)
		commandLogsFileMu.Unlock()
		if err != nil && !errors.Is(err, errNoConfigFile) {
			writeAPIError(w, http.StatusInternalServerError, &apiError{code: "command_logs_read_failed", detail: err.Error()})
			return
		}
		if runs == nil {
			runs = []json.RawMessage{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"storage": string(backendFile), "runs": runs})
	case http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, commandLogsMaxBodySize)
		var runs []json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&runs); err != nil {
			writeAPIError(w, http.StatusBadRequest, &apiError{code: "command_logs_invalid", detail: err.Error()})
			return
		}
		for _, run := range runs {
			if trimmed := bytes.TrimSpace(run); len(trimmed) == 0 || trimmed[0] != '{' {
				writeAPIError(w, http.StatusBadRequest, &apiError{code: "command_logs_invalid", detail: "every run must be an object"})
				return
			}
		}
		if runs == nil {
			runs = []json.RawMessage{}
		}
		if len(runs) > commandLogsMaxRuns {
			runs = runs[:commandLogsMaxRuns]
		}
		commandLogsFileMu.Lock()
		defer commandLogsFileMu.Unlock()
		if err := os.MkdirAll(configDir(), 0o700); err != nil {
			writeAPIError(w, http.StatusInternalServerError, &apiError{code: "command_logs_write_failed", detail: err.Error()})
			return
		}
		if err := writeJSONFile(commandLogsFilePath(), runs); err != nil {
			writeAPIError(w, http.StatusInternalServerError, &apiError{code: "command_logs_write_failed", detail: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "runs": len(runs)})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}
