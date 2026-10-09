package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func commandsStateRequest(t *testing.T, s *server, method, body, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "/api/commands/state", bytes.NewBufferString(body))
	request.RemoteAddr = remoteAddr
	response := httptest.NewRecorder()
	s.handleCommandsState(response, request)
	return response
}

func TestCommandsStateRoundTripsThroughCommandsJSON(t *testing.T) {
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	resetConfigDirCache(t)
	s := newServer()

	response := commandsStateRequest(t, s, http.MethodGet, "", "127.0.0.1:5000")
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"exists":false`)) {
		t.Fatalf("first GET = %d %s, want exists:false", response.Code, response.Body)
	}

	body := `{"flows":[{"id":"flow-1","name":"deploy","lines":[{"text":"df -h"}]}],"groups":[{"id":"group-1","name":"release","flows":[]}],"servers":[]}`
	if response := commandsStateRequest(t, s, http.MethodPut, body, "127.0.0.1:5000"); response.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", response.Code, response.Body)
	}
	if _, err := os.Stat(commandsFilePath()); err != nil {
		t.Fatalf("commands.json was not written: %v", err)
	}

	response = commandsStateRequest(t, s, http.MethodGet, "", "127.0.0.1:5000")
	var payload struct {
		Exists   bool `json:"exists"`
		Commands struct {
			Flows     []map[string]any `json:"flows"`
			Groups    []map[string]any `json:"groups"`
			Favorites []map[string]any `json:"favorites"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if !payload.Exists || len(payload.Commands.Flows) != 1 || payload.Commands.Flows[0]["name"] != "deploy" || len(payload.Commands.Groups) != 1 {
		t.Fatalf("GET after PUT = %s", response.Body)
	}
	if payload.Commands.Favorites == nil {
		t.Fatalf("a missing section should come back as an empty array: %s", response.Body)
	}
}

func TestCommandsStateRejectsNonArraySections(t *testing.T) {
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	resetConfigDirCache(t)
	s := newServer()

	response := commandsStateRequest(t, s, http.MethodPut, `{"flows":{"id":"x"}}`, "127.0.0.1:5000")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("PUT with an object for flows = %d, want 400", response.Code)
	}
	if _, err := os.Stat(commandsFilePath()); err == nil {
		t.Fatal("an invalid PUT must not create commands.json")
	}
}

func TestCommandsStateKeepsRemoteVisitorsOffTheDisk(t *testing.T) {
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	resetConfigDirCache(t)
	s := newServer()

	response := commandsStateRequest(t, s, http.MethodPut, `{"flows":[]}`, "124.174.71.198:51234")
	if response.Code != http.StatusForbidden {
		t.Fatalf("remote PUT = %d, want 403", response.Code)
	}
	if _, err := os.Stat(commandsFilePath()); err == nil {
		t.Fatal("a remote visitor must not write commands.json")
	}
	response = commandsStateRequest(t, s, http.MethodGet, "", "124.174.71.198:51234")
	if !bytes.Contains(response.Body.Bytes(), []byte(`"storage":"browser"`)) {
		t.Fatalf("remote GET = %s, want storage:browser", response.Body)
	}
}
