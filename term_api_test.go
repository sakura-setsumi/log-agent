package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// startFakeSSHServer runs an in-process SSH server that accepts one password,
// answers a PTY shell by echoing what it is sent, and serves an in-memory SFTP
// file system. It returns the address and the fingerprint of its host key.
func startFakeSSHServer(t *testing.T) (host, port, fingerprint string) {
	return startFakeSSHServerWithPassword(t, "pw")
}

func startFakeSSHServerWithPassword(t *testing.T, acceptedPassword string) (host, port, fingerprint string) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if conn.User() == "tester" && string(password) == acceptedPassword {
			return nil, nil
		}
		return nil, io.ErrUnexpectedEOF
	}}
	config.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	files := sftp.InMemHandler()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				serverConn, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				defer serverConn.Close()
				go ssh.DiscardRequests(requests)
				for newChannel := range channels {
					if newChannel.ChannelType() != "session" {
						_ = newChannel.Reject(ssh.UnknownChannelType, "session only")
						continue
					}
					channel, requests, err := newChannel.Accept()
					if err != nil {
						return
					}
					go func() {
						for request := range requests {
							switch request.Type {
							case "pty-req", "window-change":
								_ = request.Reply(true, nil)
							case "shell":
								_ = request.Reply(true, nil)
								go func() {
									_, _ = channel.Write([]byte("ready$ "))
									buf := make([]byte, 1024)
									for {
										n, err := channel.Read(buf)
										if n > 0 {
											_, _ = channel.Write(buf[:n])
										}
										if err != nil {
											return
										}
									}
								}()
							case "subsystem":
								if len(request.Payload) >= 8 && string(request.Payload[4:]) == "sftp" {
									_ = request.Reply(true, nil)
									go func() {
										_ = sftp.NewRequestServer(channel, files).Serve()
									}()
								} else {
									_ = request.Reply(false, nil)
								}
							default:
								_ = request.Reply(false, nil)
							}
						}
					}()
				}
			}()
		}
	}()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.IP.String(), strconv.Itoa(addr.Port), ssh.FingerprintSHA256(signer.PublicKey())
}

func TestTermSessionShellAndFiles(t *testing.T) {
	t.Cleanup(closeAllTermSessions)
	host, port, fingerprint := startFakeSSHServer(t)
	api := httptest.NewServer(http.HandlerFunc((&server{}).handleTerm))
	t.Cleanup(api.Close)

	post := func(path string, body []byte) *http.Response {
		t.Helper()
		response, err := http.Post(api.URL+path, "application/octet-stream", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = response.Body.Close() })
		return response
	}
	decode := func(response *http.Response, into any) {
		t.Helper()
		if err := json.NewDecoder(response.Body).Decode(into); err != nil {
			t.Fatalf("decode %s: %v", response.Request.URL.Path, err)
		}
	}
	open := func(secret, trusted string) *http.Response {
		body, _ := json.Marshal(map[string]any{"host": host, "port": port, "user": "tester", "auth": "password", "secret": secret, "fingerprint": trusted, "cols": 100, "rows": 30})
		return post("/api/term/open", body)
	}

	// An unknown host key is reported for confirmation, not trusted silently.
	first := open("pw", "")
	if first.StatusCode != http.StatusPreconditionRequired {
		t.Fatalf("first connect status = %d, want 428", first.StatusCode)
	}
	var challenge struct{ Fingerprint string }
	decode(first, &challenge)
	if challenge.Fingerprint != fingerprint {
		t.Fatalf("reported fingerprint %q, want %q", challenge.Fingerprint, fingerprint)
	}
	if wrong := open("pw", "SHA256:nope"); wrong.StatusCode != http.StatusBadGateway {
		t.Fatalf("mismatching fingerprint status = %d, want 502", wrong.StatusCode)
	}
	if bad := open("wrong-password", fingerprint); bad.StatusCode != http.StatusBadGateway {
		t.Fatalf("wrong password status = %d, want 502", bad.StatusCode)
	}

	var opened struct{ ID string }
	second := open("pw", fingerprint)
	if second.StatusCode != http.StatusOK {
		t.Fatalf("open status = %d", second.StatusCode)
	}
	decode(second, &opened)
	id := url.QueryEscape(opened.ID)

	// The shell's output arrives as a raw byte stream.
	stream, err := http.Get(api.URL + "/api/term/output?id=" + id + "&from=0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stream.Body.Close() })
	waitFor := func(want string) string {
		t.Helper()
		var got bytes.Buffer
		buf := make([]byte, 4096)
		deadline := time.Now().Add(5 * time.Second)
		for !strings.Contains(got.String(), want) {
			if time.Now().After(deadline) {
				t.Fatalf("shell output %q never contained %q", got.String(), want)
			}
			n, err := stream.Body.Read(buf)
			got.Write(buf[:n])
			if err != nil {
				t.Fatalf("stream ended with %q before %q: %v", got.String(), want, err)
			}
		}
		return got.String()
	}
	waitFor("ready$ ")
	if input := post("/api/term/input?id="+id, []byte("echo-me\n")); input.StatusCode != http.StatusNoContent {
		t.Fatalf("input status = %d", input.StatusCode)
	}
	waitFor("echo-me")
	if resize := post("/api/term/resize?id="+id+"&cols=80&rows=24", nil); resize.StatusCode != http.StatusNoContent {
		t.Fatalf("resize status = %d", resize.StatusCode)
	}

	// A reconnecting page can resume from the offset it already has.
	resumed, err := http.Get(api.URL + "/api/term/output?id=" + id + "&from=0")
	if err != nil {
		t.Fatal(err)
	}
	head := make([]byte, 7)
	if _, err := io.ReadFull(resumed.Body, head); err != nil || string(head) != "ready$ " {
		t.Fatalf("replayed output = %q (%v), want the start of the shell output", head, err)
	}
	_ = resumed.Body.Close()

	// SFTP: directories, upload, listing, download, rename and removal.
	if mk := post("/api/term/fs/mkdir?id="+id+"&path="+url.QueryEscape("/docs"), nil); mk.StatusCode != http.StatusOK {
		t.Fatalf("mkdir status = %d", mk.StatusCode)
	}
	content := []byte("hello 世界\n")
	if up := post("/api/term/fs/upload?id="+id+"&path="+url.QueryEscape("/docs/a.txt"), content); up.StatusCode != http.StatusOK {
		t.Fatalf("upload status = %d", up.StatusCode)
	}
	var listing termListResponse
	list, err := http.Get(api.URL + "/api/term/fs/list?id=" + id + "&path=" + url.QueryEscape("/"))
	if err != nil {
		t.Fatal(err)
	}
	decode(list, &listing)
	if len(listing.Entries) != 1 || listing.Entries[0].Name != "docs" || !listing.Entries[0].IsDir {
		t.Fatalf("root listing = %+v, want the docs directory", listing)
	}
	list, _ = http.Get(api.URL + "/api/term/fs/list?id=" + id + "&path=" + url.QueryEscape("/docs"))
	decode(list, &listing)
	if len(listing.Entries) != 1 || listing.Entries[0].Name != "a.txt" || listing.Entries[0].Size != int64(len(content)) {
		t.Fatalf("docs listing = %+v, want a.txt of %d bytes", listing, len(content))
	}
	download, err := http.Get(api.URL + "/api/term/fs/download?id=" + id + "&path=" + url.QueryEscape("/docs/a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(download.Body)
	_ = download.Body.Close()
	if !bytes.Equal(got, content) {
		t.Fatalf("downloaded %q, want %q", got, content)
	}
	if !strings.Contains(download.Header.Get("Content-Disposition"), "a.txt") {
		t.Fatalf("download has no file name: %q", download.Header.Get("Content-Disposition"))
	}
	if mv := post("/api/term/fs/rename?id="+id+"&from="+url.QueryEscape("/docs/a.txt")+"&to="+url.QueryEscape("/docs/b.txt"), nil); mv.StatusCode != http.StatusOK {
		t.Fatalf("rename status = %d", mv.StatusCode)
	}
	if rm := post("/api/term/fs/remove?id="+id+"&path="+url.QueryEscape("/docs"), nil); rm.StatusCode != http.StatusOK {
		t.Fatalf("remove status = %d", rm.StatusCode)
	}
	list, _ = http.Get(api.URL + "/api/term/fs/list?id=" + id + "&path=" + url.QueryEscape("/"))
	listing = termListResponse{}
	decode(list, &listing)
	if len(listing.Entries) != 0 {
		t.Fatalf("root listing after removal = %+v, want it empty", listing)
	}

	// Paths must be absolute, and the root cannot be removed.
	if bad := post("/api/term/fs/remove?id="+id+"&path="+url.QueryEscape("/"), nil); bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("removing / status = %d, want 400", bad.StatusCode)
	}
	if bad := post("/api/term/fs/mkdir?id="+id+"&path="+url.QueryEscape("relative"), nil); bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("relative path status = %d, want 400", bad.StatusCode)
	}

	// Closing ends the session and its output stream.
	if closed := post("/api/term/close?id="+id, nil); closed.StatusCode != http.StatusOK {
		t.Fatalf("close status = %d", closed.StatusCode)
	}
	if after := post("/api/term/input?id="+id, []byte("x")); after.StatusCode != http.StatusNotFound {
		t.Fatalf("input after close status = %d, want 404", after.StatusCode)
	}
	if _, err := io.Copy(io.Discard, stream.Body); err != nil {
		t.Fatalf("output stream did not end cleanly after close: %v", err)
	}
}
