package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func commandLogsRequest(t *testing.T, s *server, method, body, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "/api/commands/logs", bytes.NewBufferString(body))
	request.RemoteAddr = remoteAddr
	response := httptest.NewRecorder()
	s.handleCommandLogs(response, request)
	return response
}

func TestCommandLogsRoundTripAndCap(t *testing.T) {
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	resetConfigDirCache(t)
	s := newServer()

	if response := commandLogsRequest(t, s, http.MethodGet, "", "127.0.0.1:5000"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"runs":[]`) {
		t.Fatalf("first GET = %d %s, want an empty list", response.Code, response.Body)
	}

	runs := make([]string, commandLogsMaxRuns+5)
	for i := range runs {
		runs[i] = fmt.Sprintf(`{"id":"run-%d","entries":[{"command":"ls","output":"ok"}]}`, i)
	}
	if response := commandLogsRequest(t, s, http.MethodPut, "["+strings.Join(runs, ",")+"]", "127.0.0.1:5000"); response.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", response.Code, response.Body)
	}

	var payload struct {
		Runs []struct {
			ID string `json:"id"`
		} `json:"runs"`
	}
	response := commandLogsRequest(t, s, http.MethodGet, "", "127.0.0.1:5000")
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if len(payload.Runs) != commandLogsMaxRuns || payload.Runs[0].ID != "run-0" {
		t.Fatalf("stored %d runs starting at %q, want the newest %d", len(payload.Runs), payload.Runs[0].ID, commandLogsMaxRuns)
	}
}

func TestCommandLogsRejectsBadInputAndRemoteWrites(t *testing.T) {
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	resetConfigDirCache(t)
	s := newServer()

	if response := commandLogsRequest(t, s, http.MethodPut, `["not a run"]`, "127.0.0.1:5000"); response.Code != http.StatusBadRequest {
		t.Fatalf("PUT with a non-object run = %d, want 400", response.Code)
	}
	if response := commandLogsRequest(t, s, http.MethodPut, `[]`, "124.174.71.198:51234"); response.Code != http.StatusForbidden {
		t.Fatalf("remote PUT = %d, want 403", response.Code)
	}
	if _, err := os.Stat(commandLogsFilePath()); err == nil {
		t.Fatal("rejected writes must not create command-logs.json")
	}
	if response := commandLogsRequest(t, s, http.MethodGet, "", "124.174.71.198:51234"); !strings.Contains(response.Body.String(), `"storage":"browser"`) {
		t.Fatalf("remote GET = %s, want storage:browser", response.Body)
	}
}
