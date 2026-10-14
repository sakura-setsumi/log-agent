package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// The terminal page is a small SSH client: an interactive shell with a PTY and
// an SFTP file browser over the same connection. Both live in a session that is
// opened by /api/term/open and addressed by its id afterwards, so the page never
// has to resend credentials. The shell's output is a plain byte stream
// (/api/term/output) that the page feeds into xterm.js; keystrokes go back with
// /api/term/input.

const (
	termMaxSessions   = 16
	termReplayBytes   = 512 << 10
	termChunkBytes    = 32 << 10
	termInputLimit    = 256 << 10
	termDefaultCols   = 120
	termDefaultRows   = 32
	termKeepAlive     = 30 * time.Second
	termMaxUploadSize = 4 << 30
)

type termSession struct {
	id      string
	client  *ssh.Client
	session *ssh.Session
	stdin   io.WriteCloser

	mu   sync.Mutex
	cond *sync.Cond
	// buf holds the most recent shell output; base is the absolute offset of
	// buf[0], so a reader that reconnects can ask for "everything after N".
	buf  []byte
	base int64
	done bool

	sftpOnce sync.Once
	sftp     *sftp.Client
	sftpErr  error

	closeOnce sync.Once
	closed    chan struct{}
}

// Write receives the shell's stdout and stderr.
func (t *termSession) Write(p []byte) (int, error) {
	t.mu.Lock()
	t.buf = append(t.buf, p...)
	if extra := len(t.buf) - termReplayBytes; extra > 0 {
		t.buf = append([]byte(nil), t.buf[extra:]...)
		t.base += int64(extra)
	}
	t.cond.Broadcast()
	t.mu.Unlock()
	return len(p), nil
}

func (t *termSession) finish() {
	t.mu.Lock()
	t.done = true
	t.cond.Broadcast()
	t.mu.Unlock()
}

func (t *termSession) close() {
	t.closeOnce.Do(func() {
		close(t.closed)
		t.sftpOnce.Do(func() { t.sftpErr = errors.New("session closed") })
		_ = t.session.Close()
		// Dropping the connection first ends the SFTP channel too; closing the
		// SFTP client before that waits on a server that may never answer.
		_ = t.client.Close()
		if t.sftp != nil {
			_ = t.sftp.Close()
		}
		t.finish()
	})
}

func (t *termSession) files() (*sftp.Client, error) {
	t.sftpOnce.Do(func() { t.sftp, t.sftpErr = sftp.NewClient(t.client) })
	return t.sftp, t.sftpErr
}

type termRegistry struct {
	mu       sync.Mutex
	sessions map[string]*termSession
}

var termSessions = &termRegistry{sessions: map[string]*termSession{}}

func (r *termRegistry) add(t *termSession) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.sessions) >= termMaxSessions {
		return false
	}
	r.sessions[t.id] = t
	return true
}

func (r *termRegistry) get(id string) *termSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessions[id]
}

func (r *termRegistry) remove(id string) *termSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.sessions[id]
	delete(r.sessions, id)
	return t
}

func newTermID() string {
	var raw [12]byte
	_, _ = rand.Read(raw[:])
	return hex.EncodeToString(raw[:])
}

type termOpenRequest struct {
	commandConnectionRequest
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

func (s *server) handleTerm(w http.ResponseWriter, r *http.Request) {
	action := strings.TrimPrefix(r.URL.Path, "/api/term/")
	switch action {
	case "open":
		s.handleTermOpen(w, r)
		return
	}
	t := termSessions.get(r.URL.Query().Get("id"))
	if t == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session_not_found"})
		return
	}
	switch action {
	case "output":
		handleTermOutput(w, r, t)
	case "input":
		handleTermInput(w, r, t)
	case "resize":
		handleTermResize(w, r, t)
	case "close":
		termSessions.remove(t.id)
		t.close()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case "fs/list":
		handleTermList(w, r, t)
	case "fs/download":
		handleTermDownload(w, r, t)
	case "fs/upload":
		handleTermUpload(w, r, t)
	case "fs/mkdir":
		handleTermMkdir(w, r, t)
	case "fs/remove":
		handleTermRemove(w, r, t)
	case "fs/rename":
		handleTermRename(w, r, t)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	}
}

func (s *server) handleTermOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var request termOpenRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	request.Host = strings.TrimSpace(request.Host)
	request.Port = strings.TrimSpace(request.Port)
	request.User = strings.TrimSpace(request.User)
	request.Auth = strings.TrimSpace(request.Auth)
	request.Fingerprint = strings.TrimSpace(request.Fingerprint)
	if request.Port == "" {
		request.Port = "22"
	}
	if port, err := strconv.Atoi(request.Port); err != nil || port < 1 || port > 65535 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_port"})
		return
	}
	switch {
	case request.Host == "":
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_host"})
		return
	case request.User == "":
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_user"})
		return
	case strings.TrimSpace(request.Secret) == "":
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_secret"})
		return
	}
	if request.Cols < 2 || request.Cols > 1000 {
		request.Cols = termDefaultCols
	}
	if request.Rows < 1 || request.Rows > 1000 {
		request.Rows = termDefaultRows
	}

	client, err := newCommandSSHClient(request.commandConnectionRequest)
	if err != nil {
		var hostKeyError *commandHostKeyError
		if errors.As(err, &hostKeyError) {
			writeJSON(w, http.StatusPreconditionRequired, map[string]string{"error": hostKeyError.Error(), "fingerprint": hostKeyError.Fingerprint})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "connect_failed", "detail": err.Error()})
		return
	}
	session, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "session_failed", "detail": err.Error()})
		return
	}
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 38400, ssh.TTY_OP_OSPEED: 38400}
	if err := session.RequestPty("xterm-256color", request.Rows, request.Cols, modes); err != nil {
		_ = session.Close()
		_ = client.Close()
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "pty_failed", "detail": err.Error()})
		return
	}
	t := &termSession{id: newTermID(), client: client, session: session, closed: make(chan struct{})}
	t.cond = sync.NewCond(&t.mu)
	stdin, err := session.StdinPipe()
	if err != nil {
		t.close()
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "session_failed", "detail": err.Error()})
		return
	}
	t.stdin = stdin
	session.Stdout = t
	session.Stderr = t
	if err := session.Shell(); err != nil {
		t.close()
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "shell_failed", "detail": err.Error()})
		return
	}
	if !termSessions.add(t) {
		t.close()
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too_many_sessions"})
		return
	}
	go func() {
		_ = session.Wait()
		t.finish()
		_ = client.Close()
	}()
	go func() {
		ticker := time.NewTicker(termKeepAlive)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
					return
				}
			case <-t.closed:
				return
			}
		}
	}()
	writeJSON(w, http.StatusOK, map[string]any{"id": t.id})
}

// handleTermOutput streams the shell's output as raw bytes. The X-Term-Offset
// header tells the page which absolute offset the first byte has; a page that
// lost the stream asks again with ?from=<offset it already has>.
func handleTermOutput(w http.ResponseWriter, r *http.Request, t *termSession) {
	from, _ := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	ctx := r.Context()
	go func() {
		<-ctx.Done()
		t.mu.Lock()
		t.cond.Broadcast()
		t.mu.Unlock()
	}()

	t.mu.Lock()
	if from < t.base {
		from = t.base
	}
	t.mu.Unlock()

	flusher, _ := w.(http.Flusher)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Term-Offset", strconv.FormatInt(from, 10))
	w.WriteHeader(http.StatusOK)
	if flusher != nil {
		flusher.Flush()
	}
	for {
		t.mu.Lock()
		for from >= t.base+int64(len(t.buf)) && !t.done && ctx.Err() == nil {
			t.cond.Wait()
		}
		if ctx.Err() != nil {
			t.mu.Unlock()
			return
		}
		if from < t.base {
			from = t.base
		}
		end := t.base + int64(len(t.buf))
		if from >= end {
			t.mu.Unlock()
			return // the shell has ended and everything was delivered
		}
		stop := from + termChunkBytes
		if stop > end {
			stop = end
		}
		chunk := append([]byte(nil), t.buf[from-t.base:stop-t.base]...)
		t.mu.Unlock()
		if _, err := w.Write(chunk); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
		from = stop
	}
}

func handleTermInput(w http.ResponseWriter, r *http.Request, t *termSession) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, termInputLimit))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if _, err := t.stdin.Write(data); err != nil {
		writeJSON(w, http.StatusGone, map[string]string{"error": "session_closed"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleTermResize(w http.ResponseWriter, r *http.Request, t *termSession) {
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))
	if cols < 2 || cols > 1000 || rows < 1 || rows > 1000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_size"})
		return
	}
	if err := t.session.WindowChange(rows, cols); err != nil {
		writeJSON(w, http.StatusGone, map[string]string{"error": "session_closed"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type termFileEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime int64  `json:"modTime"`
	IsDir   bool   `json:"isDir"`
	IsLink  bool   `json:"isLink"`
	UID     uint32 `json:"uid"`
	GID     uint32 `json:"gid"`
}

type termListResponse struct {
	Path    string          `json:"path"`
	Entries []termFileEntry `json:"entries"`
}

func termFSError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	code := "sftp_failed"
	switch {
	case errors.Is(err, os.ErrNotExist):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, os.ErrPermission):
		status, code = http.StatusForbidden, "permission_denied"
	case errors.Is(err, os.ErrExist):
		status, code = http.StatusConflict, "already_exists"
	}
	writeJSON(w, status, map[string]string{"error": code, "detail": err.Error()})
}

// termPath turns a page-supplied path into a clean absolute remote path.
func termPath(raw string) (string, bool) {
	if strings.ContainsRune(raw, 0) {
		return "", false
	}
	raw = strings.ReplaceAll(raw, "\\", "/")
	if !strings.HasPrefix(raw, "/") {
		return "", false
	}
	return pathpkg.Clean(raw), true
}

func handleTermList(w http.ResponseWriter, r *http.Request, t *termSession) {
	client, err := t.files()
	if err != nil {
		termFSError(w, err)
		return
	}
	dir := r.URL.Query().Get("path")
	if strings.TrimSpace(dir) == "" {
		home, err := client.Getwd()
		if err != nil {
			home = "/"
		}
		dir = home
	}
	dir, ok := termPath(dir)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_path"})
		return
	}
	infos, err := client.ReadDir(dir)
	if err != nil {
		termFSError(w, err)
		return
	}
	entries := make([]termFileEntry, 0, len(infos))
	for _, info := range infos {
		entry := termFileEntry{Name: info.Name(), Size: info.Size(), Mode: info.Mode().String(), ModTime: info.ModTime().Unix(), IsDir: info.IsDir()}
		if info.Mode()&os.ModeSymlink != 0 {
			entry.IsLink = true
			if target, err := client.Stat(pathpkg.Join(dir, info.Name())); err == nil {
				entry.IsDir = target.IsDir()
				entry.Size = target.Size()
			}
		}
		if stat, ok := info.Sys().(*sftp.FileStat); ok {
			entry.UID, entry.GID = stat.UID, stat.GID
		}
		entries = append(entries, entry)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	writeJSON(w, http.StatusOK, termListResponse{Path: dir, Entries: entries})
}

func handleTermDownload(w http.ResponseWriter, r *http.Request, t *termSession) {
	client, err := t.files()
	if err != nil {
		termFSError(w, err)
		return
	}
	target, ok := termPath(r.URL.Query().Get("path"))
	if !ok || target == "/" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_path"})
		return
	}
	file, err := client.Open(target)
	if err != nil {
		termFSError(w, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		termFSError(w, err)
		return
	}
	if info.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "is_directory"})
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(pathpkg.Base(target)))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, file)
}

func handleTermUpload(w http.ResponseWriter, r *http.Request, t *termSession) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	// Reporting before the body is read makes browsers show a bare network
	// error, so a failure drains what is left of the upload first.
	fail := func(status int, payload map[string]string) {
		_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 1<<20))
		writeJSON(w, status, payload)
	}
	client, err := t.files()
	if err != nil {
		termFSError(w, err)
		return
	}
	target, ok := termPath(r.URL.Query().Get("path"))
	if !ok || target == "/" {
		fail(http.StatusBadRequest, map[string]string{"error": "invalid_path"})
		return
	}
	if info, err := client.Stat(target); err == nil && info.IsDir() {
		fail(http.StatusBadRequest, map[string]string{"error": "is_directory"})
		return
	}
	file, err := client.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		termFSError(w, err)
		return
	}
	written, copyErr := io.Copy(file, http.MaxBytesReader(w, r.Body, termMaxUploadSize))
	closeErr := file.Close()
	if copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upload_failed", "detail": copyErr.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "size": written})
}

func handleTermMkdir(w http.ResponseWriter, r *http.Request, t *termSession) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	client, err := t.files()
	if err != nil {
		termFSError(w, err)
		return
	}
	target, ok := termPath(r.URL.Query().Get("path"))
	if !ok || target == "/" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_path"})
		return
	}
	if err := client.MkdirAll(target); err != nil {
		termFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func handleTermRemove(w http.ResponseWriter, r *http.Request, t *termSession) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	client, err := t.files()
	if err != nil {
		termFSError(w, err)
		return
	}
	target, ok := termPath(r.URL.Query().Get("path"))
	if !ok || target == "/" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_path"})
		return
	}
	info, err := client.Lstat(target)
	if err != nil {
		termFSError(w, err)
		return
	}
	if info.IsDir() {
		err = client.RemoveAll(target)
	} else {
		err = client.Remove(target)
	}
	if err != nil {
		termFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func handleTermRename(w http.ResponseWriter, r *http.Request, t *termSession) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	client, err := t.files()
	if err != nil {
		termFSError(w, err)
		return
	}
	from, okFrom := termPath(r.URL.Query().Get("from"))
	to, okTo := termPath(r.URL.Query().Get("to"))
	if !okFrom || !okTo || from == "/" || to == "/" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_path"})
		return
	}
	if _, err := client.Lstat(to); err == nil {
		termFSError(w, fmt.Errorf("%s: %w", to, os.ErrExist))
		return
	}
	if err := client.Rename(from, to); err != nil {
		termFSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// closeAllTermSessions ends every open shell; used by tests.
func closeAllTermSessions() {
	termSessions.mu.Lock()
	sessions := make([]*termSession, 0, len(termSessions.sessions))
	for _, t := range termSessions.sessions {
		sessions = append(sessions, t)
	}
	termSessions.sessions = map[string]*termSession{}
	termSessions.mu.Unlock()
	for _, t := range sessions {
		t.close()
	}
}
