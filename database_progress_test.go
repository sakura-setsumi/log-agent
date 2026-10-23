package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDatabaseFetchingProgress(t *testing.T) {
	a, b := databaseTestSetup(t)
	id := fmt.Sprintf("fetching-%d", time.Now().UnixNano())
	result := databaseDecode[databaseQueryResponse](t, databaseRequest(t, "query?request_id="+id, nil, map[string]any{"targets": []databaseTarget{a, b}, "sql": "SELECT * FROM orders ORDER BY id", "limit": 1}))
	if len(result.Rows) != 2 {
		t.Fatalf("merged rows: %d", len(result.Rows))
	}
	var progress struct {
		Records int   `json:"records"`
		Done    bool  `json:"done"`
		Elapsed int64 `json:"elapsedMs"`
	}
	progress = databaseDecode[struct {
		Records int   `json:"records"`
		Done    bool  `json:"done"`
		Elapsed int64 `json:"elapsedMs"`
	}](t, databaseRequest(t, "progress?request_id="+id, nil, nil))
	if progress.Records != 2 || !progress.Done || progress.Elapsed < 0 {
		t.Fatalf("progress: %+v", progress)
	}
	// LIMIT's lookahead and last-page COUNT are not counted as fetched records.
	tableID := id + "-table"
	table := databaseRequest(t, "table?request_id="+tableID+"&id="+a.ID+"&database=main&table=orders&limit=1&last=1", nil, nil)
	if table.Code != 200 {
		t.Fatal(table.Body.String())
	}
	tableProgress := databaseDecode[map[string]any](t, databaseRequest(t, "progress?request_id="+tableID, nil, nil))
	if tableProgress["records"] != float64(1) {
		t.Fatalf("table count: %v", tableProgress)
	}
	duplicate := databaseRequest(t, "query?request_id="+id, nil, map[string]any{"targets": []databaseTarget{a}, "sql": "SELECT * FROM orders"})
	if duplicate.Code != http.StatusBadRequest {
		t.Fatalf("duplicate progress request: %d", duplicate.Code)
	}
	missing := databaseRequest(t, "progress?request_id=missing-"+id, nil, nil)
	if missing.Code != 404 {
		t.Fatalf("unknown progress: %d", missing.Code)
	}
	remote := httptest.NewRequest(http.MethodGet, "/api/database/progress?request_id="+id, nil)
	remote.RemoteAddr = "203.0.113.2:1000"
	denied := httptest.NewRecorder()
	(&server{}).handleDatabase(denied, remote)
	if denied.Code == 200 {
		t.Fatal("remote client read local progress")
	}
}

func TestDatabaseProgressExpiry(t *testing.T) {
	id := fmt.Sprintf("expiry-%d", time.Now().UnixNano())
	progress, err := beginDatabaseProgress(id)
	if err != nil {
		t.Fatal(err)
	}
	progress.rows.Add(7)
	live := databaseDecode[map[string]any](t, databaseRequest(t, "progress?request_id="+id, nil, nil))
	if live["records"] != float64(7) || live["done"] != false {
		t.Fatalf("live progress: %v", live)
	}
	finishDatabaseProgress(progress)
	databaseProgressLoads.Lock()
	progress.completed = time.Now().Add(-2 * time.Minute)
	databaseProgressLoads.Unlock()
	next, err := beginDatabaseProgress(id + "-next")
	if err != nil {
		t.Fatal(err)
	}
	defer finishDatabaseProgress(next)
	if databaseRequest(t, "progress?request_id="+id, nil, nil).Code != 404 {
		t.Fatal("completed observation was not expired")
	}
}
