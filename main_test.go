package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
		nodes:          []Node{{ID: "node-1", Name: "node", baseURL: dozzle.URL}},
		logs:           []LogEntry{},
		historyRange:   "1d",
		historyGeneration: 0,
		rules:          map[string]bool{},
		containerNames: map[string]map[string]string{"node-1": {"container-1": "api"}},
		containerLogs:  map[string][]LogEntry{},
		subscribers:    map[chan LogEntry]struct{}{},
		streams:        map[string]struct{}{},
		nodeContexts:   map[string]context.Context{},
		nodeCancels:    map[string]context.CancelFunc{},
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
