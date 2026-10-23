package main

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

type databaseProgressKey struct{}
type databaseLoadProgress struct {
	started   time.Time
	rows      atomic.Int64
	completed time.Time // guarded by databaseProgressLoads
}

var databaseProgressLoads = struct {
	sync.Mutex
	items map[string]*databaseLoadProgress
}{items: make(map[string]*databaseLoadProgress)}
var databaseProgressID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

func beginDatabaseProgress(id string) (*databaseLoadProgress, error) {
	if !databaseProgressID.MatchString(id) {
		return nil, errors.New("invalid progress request ID")
	}
	databaseProgressLoads.Lock()
	defer databaseProgressLoads.Unlock()
	now := time.Now()
	for key, item := range databaseProgressLoads.items {
		if !item.completed.IsZero() && now.Sub(item.completed) > time.Minute || now.Sub(item.started) > 5*time.Minute {
			delete(databaseProgressLoads.items, key)
		}
	}
	if _, exists := databaseProgressLoads.items[id]; exists {
		return nil, errors.New("progress request ID already exists")
	}
	if len(databaseProgressLoads.items) >= 128 {
		// Completed observations can be evicted; active requests stay isolated.
		for key, item := range databaseProgressLoads.items {
			if !item.completed.IsZero() {
				delete(databaseProgressLoads.items, key)
			}
		}
	}
	if len(databaseProgressLoads.items) >= 128 {
		return nil, errors.New("too many active database requests")
	}
	progress := &databaseLoadProgress{started: now}
	databaseProgressLoads.items[id] = progress
	return progress, nil
}
func finishDatabaseProgress(progress *databaseLoadProgress) {
	databaseProgressLoads.Lock()
	defer databaseProgressLoads.Unlock()
	progress.completed = time.Now()
}
func databaseProgressFromContext(ctx context.Context) *databaseLoadProgress {
	progress, _ := ctx.Value(databaseProgressKey{}).(*databaseLoadProgress)
	return progress
}
func handleDatabaseProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	databaseProgressLoads.Lock()
	defer databaseProgressLoads.Unlock()
	progress := databaseProgressLoads.items[r.URL.Query().Get("request_id")]
	w.Header().Set("Cache-Control", "no-store")
	if progress == nil {
		databaseError(w, http.StatusNotFound, errors.New("database request not found"))
		return
	}
	end := time.Now()
	if !progress.completed.IsZero() {
		end = progress.completed
	}
	writeJSON(w, 200, map[string]any{"records": progress.rows.Load(), "elapsedMs": end.Sub(progress.started).Milliseconds(), "done": !progress.completed.IsZero()})
}
