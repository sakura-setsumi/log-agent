package main

import (
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type bridgeRecorder struct {
	mu      sync.Mutex
	scripts []string
	changed chan struct{}
}

func TestBridgeCloseCancelsWindowRequests(t *testing.T) {
	started, cancelled := make(chan struct{}), make(chan struct{})
	b := newBridge(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(cancelled)
	}), func(string) {})
	b.request("stream", "GET", "/api/stream", nil)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	b.body("upload", base64.StdEncoding.EncodeToString([]byte("pending")))
	b.close()
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("window close did not cancel its stream")
	}
	b.body("late-upload", base64.StdEncoding.EncodeToString([]byte("ignored")))
	b.request("late-request", "GET", "/api/stream", nil)
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed || len(b.bodies) != 0 {
		t.Fatal("closed bridge retained uploads or accepted a late request")
	}
}

func newBridgeRecorder() *bridgeRecorder { return &bridgeRecorder{changed: make(chan struct{}, 64)} }

func (r *bridgeRecorder) emit(script string) {
	r.mu.Lock()
	r.scripts = append(r.scripts, script)
	r.mu.Unlock()
	r.changed <- struct{}{}
}

func (r *bridgeRecorder) waitFor(t *testing.T, prefix string) string {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		r.mu.Lock()
		for _, script := range r.scripts {
			if strings.HasPrefix(script, prefix) {
				r.mu.Unlock()
				return script
			}
		}
		r.mu.Unlock()
		select {
		case <-r.changed:
		case <-deadline:
			t.Fatalf("no script starting with %q; got %q", prefix, r.scripts)
		}
	}
}

func TestBridgeDeliversBodyAndResponse(t *testing.T) {
	recorder := newBridgeRecorder()
	b := newBridge(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !isLoopbackRequest(r) {
			t.Errorf("bridge request is not loopback: %s", r.RemoteAddr)
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(r.Method + " " + r.URL.RequestURI() + " " + r.Header.Get("X-Test") + " " + string(body)))
	}), recorder.emit)

	b.body("1", base64.StdEncoding.EncodeToString([]byte("he")))
	b.body("1", base64.StdEncoding.EncodeToString([]byte("llo")))
	b.request("1", "post", "/api/x?a=1", map[string]string{"X-Test": "yes"})

	head := recorder.waitFor(t, `window.__lb.head("1",201,`)
	if !strings.Contains(head, `"Content-Type":"text/plain"`) {
		t.Fatalf("head lost the content type: %s", head)
	}
	chunk := recorder.waitFor(t, `window.__lb.chunk("1",`)
	encoded := strings.Trim(strings.TrimPrefix(chunk, `window.__lb.chunk("1",`), `")`)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || string(decoded) != "POST /api/x?a=1 yes hello" {
		t.Fatalf("chunk = %q (%v)", decoded, err)
	}
	recorder.waitFor(t, `window.__lb.end("1","")`)
}

func TestBridgeStreamsUntilCancelled(t *testing.T) {
	recorder := newBridgeRecorder()
	stopped := make(chan struct{})
	b := newBridge(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: 1\n\n"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(stopped)
	}), recorder.emit)

	b.request("s", "GET", "/api/stream", nil)
	// The head and first event arrive while the handler is still running.
	recorder.waitFor(t, `window.__lb.head("s",200,`)
	recorder.waitFor(t, `window.__lb.chunk("s",`)
	b.cancel("s")
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not stop the handler")
	}
	recorder.waitFor(t, `window.__lb.end("s","")`)
}

func TestBridgeReportsHandlerPanic(t *testing.T) {
	recorder := newBridgeRecorder()
	b := newBridge(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }), recorder.emit)
	b.request("p", "GET", "/api/x", nil)
	end := recorder.waitFor(t, `window.__lb.end("p",`)
	if !strings.Contains(end, "boom") {
		t.Fatalf("panic not reported to the page: %s", end)
	}
}

func TestBridgeRejectsNonRootPath(t *testing.T) {
	recorder := newBridgeRecorder()
	b := newBridge(http.NotFoundHandler(), recorder.emit)
	b.request("r", "GET", "http://evil.example/api", nil)
	recorder.waitFor(t, `window.__lb.end("r","invalid request path`)
}

func TestServeStaticReturnsEmbeddedIndex(t *testing.T) {
	status, header, body := serveStatic(newHTTPHandler(&server{}), "/")
	if status != http.StatusOK || !strings.Contains(header.Get("Content-Type"), "text/html") || len(body) == 0 {
		t.Fatalf("index: status %d, type %q, %d bytes", status, header.Get("Content-Type"), len(body))
	}
}
