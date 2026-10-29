package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// The desktop window has no network listener. The page reaches the Go handlers
// through this bridge instead: bridge.js replaces fetch, XMLHttpRequest and
// EventSource for same-origin /api/ URLs with calls into the functions below,
// and the response comes back as head/chunk/end messages so streaming
// endpoints (/api/stream) keep working.

const (
	// bridgeOrigin is the virtual origin the window loads. Nothing listens
	// there: the webview answers its requests from the embedded frontend.
	bridgeOrigin = "https://app.local"

	// bridgeFlushSize bounds how much response data is held back before it is
	// sent to the page. Streaming handlers call Flush and are not affected.
	bridgeFlushSize = 64 << 10
)

type bridge struct {
	handler http.Handler
	// emit runs a script in the page. It must be safe to call from any goroutine.
	emit func(script string)

	mu      sync.Mutex
	closed  bool
	bodies  map[string]*bytes.Buffer
	cancels map[string]context.CancelFunc
}

func newBridge(handler http.Handler, emit func(script string)) *bridge {
	return &bridge{
		handler: handler,
		emit:    emit,
		bodies:  map[string]*bytes.Buffer{},
		cancels: map[string]context.CancelFunc{},
	}
}

// body buffers one base64 chunk of a request body that is still being uploaded.
func (b *bridge) body(id, chunk string) {
	data, err := base64.StdEncoding.DecodeString(chunk)
	if err != nil {
		log.Printf("bridge: bad body chunk for request %s: %v", id, err)
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	buf := b.bodies[id]
	if buf == nil {
		buf = &bytes.Buffer{}
		b.bodies[id] = buf
	}
	buf.Write(data)
}

// request runs one API call. It returns at once; the answer is delivered
// through emit, so a slow handler never stalls the window's message loop.
func (b *bridge) request(id, method, target string, headers map[string]string) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	buf := b.bodies[id]
	delete(b.bodies, id)
	ctx, cancel := context.WithCancel(context.Background())
	b.cancels[id] = cancel
	b.mu.Unlock()

	var payload []byte
	if buf != nil {
		payload = buf.Bytes()
	}
	go func() {
		defer func() {
			cancel()
			b.mu.Lock()
			delete(b.cancels, id)
			b.mu.Unlock()
		}()
		w := &bridgeWriter{bridge: b, id: id, header: http.Header{}}
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("bridge: %s %s panicked: %v", method, target, recovered)
				w.finish(fmt.Sprintf("internal error: %v", recovered))
			}
		}()
		req, err := newBridgeRequest(ctx, method, target, headers, payload)
		if err != nil {
			w.finish(err.Error())
			return
		}
		b.handler.ServeHTTP(w, req)
		w.finish("")
	}()
}

// cancel aborts a running call, e.g. when the page closes an EventSource.
func (b *bridge) cancel(id string) {
	b.mu.Lock()
	cancel := b.cancels[id]
	delete(b.bodies, id)
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (b *bridge) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for _, cancel := range b.cancels {
		cancel()
	}
	clear(b.bodies)
}

func newBridgeRequest(ctx context.Context, method, target string, headers map[string]string, payload []byte) (*http.Request, error) {
	if !strings.HasPrefix(target, "/") {
		return nil, fmt.Errorf("invalid request path %q", target)
	}
	var reader io.Reader
	if len(payload) > 0 {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), bridgeOrigin+target, reader)
	if err != nil {
		return nil, err
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	// The window is the only client, and it runs on this machine.
	req.RemoteAddr = "127.0.0.1:0"
	return req, nil
}

// serveStatic answers a GET for the embedded frontend without any bridging.
func serveStatic(handler http.Handler, target string) (int, http.Header, []byte) {
	req := httptest.NewRequest(http.MethodGet, bridgeOrigin+target, nil)
	req.RemoteAddr = "127.0.0.1:0"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Code, rec.Header(), rec.Body.Bytes()
}

// bridgeWriter is the ResponseWriter handed to the handlers. It sends the head
// on the first flush (or at the end) and body data in chunks.
type bridgeWriter struct {
	bridge *bridge
	id     string
	header http.Header
	status int
	sent   bool // head delivered
	done   bool
	buf    bytes.Buffer
}

func (w *bridgeWriter) Header() http.Header { return w.header }

func (w *bridgeWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *bridgeWriter) Write(p []byte) (int, error) {
	if w.done {
		return 0, io.ErrClosedPipe
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.buf.Write(p)
	if w.buf.Len() >= bridgeFlushSize {
		w.Flush()
	}
	return len(p), nil
}

func (w *bridgeWriter) Flush() {
	if w.done {
		return
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if !w.sent {
		w.sent = true
		headers := map[string]string{}
		for name, values := range w.header {
			headers[name] = strings.Join(values, ", ")
		}
		w.bridge.emit(fmt.Sprintf("window.__lb.head(%s,%d,%s)", jsonString(w.id), w.status, jsonString(headers)))
	}
	if w.buf.Len() > 0 {
		w.bridge.emit(fmt.Sprintf("window.__lb.chunk(%s,%s)", jsonString(w.id), jsonString(base64.StdEncoding.EncodeToString(w.buf.Bytes()))))
		w.buf.Reset()
	}
}

// finish delivers whatever is pending and closes the response. A non-empty
// message reports a failure to the page.
func (w *bridgeWriter) finish(message string) {
	if w.done {
		return
	}
	if message == "" || w.sent {
		w.Flush()
	}
	w.done = true
	w.bridge.emit(fmt.Sprintf("window.__lb.end(%s,%s)", jsonString(w.id), jsonString(message)))
}

func jsonString(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return string(encoded)
}
