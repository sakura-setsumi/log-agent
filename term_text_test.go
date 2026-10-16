package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestTermTextEncoding(t *testing.T) {
	for _, encoding := range []string{"UTF-8", "UTF-8 BOM", "UTF-16 LE", "UTF-16 BE", "GB18030"} {
		text := "# 中文代码\r\necho \"你好\" 😀\r\n"
		data, err := termEncodeText(text, encoding)
		if err != nil {
			t.Fatal(err)
		}
		decoded, detected, err := termDecodeText(data, false)
		if err != nil || decoded != text || detected != encoding {
			t.Fatalf("encoding=%s detected=%s text=%q err=%v", encoding, detected, decoded, err)
		}
	}
	for _, data := range [][]byte{[]byte("abc\x00def"), {0xff, 0xfe, 0x00, 0xd8}} {
		if _, _, err := termDecodeText(data, false); err == nil {
			t.Fatal("binary or malformed text accepted")
		}
	}
	if text, _, err := termDecodeText([]byte{'a', 0xe4, 0xb8}, true); err != nil || text != "a" {
		t.Fatalf("truncated UTF-8: %q %v", text, err)
	}
}
func TestTermTextReadAndSave(t *testing.T) {
	t.Cleanup(closeAllTermSessions)
	host, port, fingerprint := startFakeSSHServer(t)
	web := httptest.NewServer(http.HandlerFunc((&server{}).handleTerm))
	defer web.Close()
	request := func(method, path string, body []byte) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, web.URL+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	config, _ := json.Marshal(map[string]any{"host": host, "port": port, "user": "tester", "auth": "password", "secret": "pw", "fingerprint": fingerprint})
	opened := request("POST", "/api/term/open", config)
	var session struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(opened.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	opened.Body.Close()
	if opened.StatusCode != 200 || session.ID == "" {
		t.Fatal("open session failed")
	}
	endpoint := func(action, path string) string {
		return "/api/term/fs/" + action + "?" + url.Values{"id": {session.ID}, "path": {path}}.Encode()
	}
	upload := func(path string, data []byte) {
		t.Helper()
		res := request("POST", endpoint("upload", path), data)
		defer res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("upload %s: %d", path, res.StatusCode)
		}
	}
	read := func(path string) termTextDocument {
		t.Helper()
		res := request("GET", endpoint("text", path), nil)
		defer res.Body.Close()
		var doc termTextDocument
		if res.StatusCode != 200 {
			data, _ := io.ReadAll(res.Body)
			t.Fatalf("read %s: %d %s", path, res.StatusCode, data)
		}
		if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
			t.Fatal(err)
		}
		return doc
	}
	original := "#!/bin/sh\r\n# 中文\r\necho \"hello\"\r\n"
	upload("/demo.sh", []byte(original))
	doc := read("/demo.sh")
	if doc.Text != original || doc.ReadOnly || doc.Encoding != "UTF-8" || len(doc.ETag) != 64 {
		t.Fatalf("document=%+v", doc)
	}
	modified := strings.ReplaceAll(original, "hello", "edited")
	body, _ := json.Marshal(map[string]string{"text": modified, "encoding": doc.Encoding, "etag": doc.ETag})
	saved := request("PUT", endpoint("text", "/demo.sh"), body)
	if saved.StatusCode != 200 {
		data, _ := io.ReadAll(saved.Body)
		t.Fatalf("save: %d %s", saved.StatusCode, data)
	}
	saved.Body.Close()
	if got := read("/demo.sh"); got.Text != modified || got.ETag == doc.ETag {
		t.Fatalf("saved document=%+v", got)
	}
	stale := request("PUT", endpoint("text", "/demo.sh"), body)
	stale.Body.Close()
	if stale.StatusCode != 409 {
		t.Fatalf("stale save=%d", stale.StatusCode)
	}
	if got := read("/demo.sh"); got.Text != modified {
		t.Fatal("conflict overwrote the file")
	}
	upload("/image.bin", []byte{0, 1, 2, 3})
	binary := request("GET", endpoint("text", "/image.bin"), nil)
	binary.Body.Close()
	if binary.StatusCode != 415 {
		t.Fatalf("binary read=%d", binary.StatusCode)
	}
	upload("/large.log", bytes.Repeat([]byte("log line\n"), termTextLimit/8+1))
	preview := read("/large.log")
	if !preview.Truncated || !preview.ReadOnly || len(preview.Text) > termTextLimit {
		t.Fatal("large file was not bounded and read-only")
	}
	for _, encoding := range []string{"UTF-8 BOM", "UTF-16 LE", "UTF-16 BE", "GB18030"} {
		data, _ := termEncodeText("中文\r\n", encoding)
		upload("/encoded.txt", data)
		doc := read("/encoded.txt")
		body, _ := json.Marshal(map[string]string{"text": "修改后\r\n", "encoding": doc.Encoding, "etag": doc.ETag})
		res := request("PUT", endpoint("text", "/encoded.txt"), body)
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("save %s: %d", encoding, res.StatusCode)
		}
		if got := read("/encoded.txt"); got.Encoding != encoding || got.Text != "修改后\r\n" {
			t.Fatalf("encoding changed: %+v", got)
		}
	}
}
