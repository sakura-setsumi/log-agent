//go:build windows

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Exercise the embedded application rather than a minimal HTML fixture, with
// active bootstrap/stream requests while a second STA creates a shared WebView.
func TestNativeDetachedFullPageCloseChild(t *testing.T) {
	if os.Getenv("LOG_AGENT_NATIVE_WINDOWS_TEST") != "1" {
		t.Skip("opt-in WebView2 integration test")
	}
	databaseTestSetup(t)
	events := make(chan string, 20)
	probe := make(chan struct{})
	defer close(probe)
	app := newHTTPHandler(newServer())
	script := `<script>addEventListener('DOMContentLoaded',async()=>{
 const report=name=>fetch('/api/window-test/event?name='+encodeURIComponent(name));
 const wait=async fn=>{const until=Date.now()+20000;while(!fn()){if(Date.now()>until)throw Error('Page initialization timeout');await new Promise(r=>setTimeout(r,50));}};
 try {
  if(new URLSearchParams(location.search).get('detached')==='commands'){
   await wait(()=>!!document.querySelector('#commands-view:not(.hidden)'));
   await report('parent-ready');
   await window.__lbOpenView('database',-20000,-20000);
   await report('window-created');
   await fetch('/api/window-test/probe');
   const profiles=await (await fetch('/api/database/profiles')).json();
   if(!profiles.some(p=>p.id==='east'))throw Error('Parent backend failed after child close');
   await report('parent-alive');
  }else{
   await wait(()=>!!document.querySelector('#db-sql-editor .cm-editor')&&!!document.querySelector('[data-db-connect="east"]'));
   const profiles=await (await fetch('/api/database/profiles')).json();
   if(!profiles.some(p=>p.id==='east'))throw Error('Child backend failed');
   await report('child-loaded');
  }
 }catch(e){await report('error:'+String(e));}
});</script>`
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/window-test/event" {
			events <- r.URL.Query().Get("name")
			fmt.Fprint(w, `{}`)
			return
		}
		if r.URL.Path == "/api/window-test/probe" {
			select {
			case <-probe:
			case <-r.Context().Done():
			}
			fmt.Fprint(w, `{}`)
			return
		}
		if r.URL.Path == "/" {
			rec := httptest.NewRecorder()
			app.ServeHTTP(rec, r)
			for key, values := range rec.Header() {
				if key != "Content-Length" {
					w.Header()[key] = values
				}
			}
			w.WriteHeader(rec.Code)
			fmt.Fprint(w, strings.Replace(rec.Body.String(), "</body>", script+"</body>", 1))
			return
		}
		app.ServeHTTP(w, r)
	})
	dataPath := filepath.Join(t.TempDir(), "webview2")
	t.Cleanup(func() {
		deadline := time.Now().Add(8 * time.Second)
		for {
			err := os.RemoveAll(dataPath)
			if err == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Errorf("release WebView2 profile: %v", err)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
	manager := &desktopWindows{handler: handler, dataPath: dataPath, done: make(chan struct{})}
	defer func() {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		for hwnd := range manager.handles {
			user32.NewProc("PostMessageW").Call(hwnd, 0x0010, 0, 0)
		}
	}()
	if err := manager.open("commands", -20000, -20000); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	deadline := time.After(30 * time.Second)
	for !seen["child-loaded"] || !seen["window-created"] {
		select {
		case event := <-events:
			if strings.HasPrefix(event, "error:") {
				t.Fatal(event)
			}
			seen[event] = true
		case <-deadline:
			t.Fatalf("full page did not initialize: %v", seen)
		}
	}
	findWindow := func(view string) uintptr {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		for hwnd, name := range manager.handles {
			if name == view {
				return hwnd
			}
		}
		return 0
	}
	child := findWindow("database")
	if child == 0 {
		t.Fatal("child window missing")
	}
	// This is the same WM_CLOSE path as the native title-bar close button.
	procSendMessage.Call(child, 0x0010, 0, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	for {
		manager.mu.Lock()
		active := manager.active
		manager.mu.Unlock()
		if active == 1 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("closing child did not leave parent alive")
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}
	probe <- struct{}{}
	select {
	case event := <-events:
		if event != "parent-alive" {
			t.Fatalf("unexpected parent state: %s", event)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("parent stopped responding after child closed")
	}
	parent := findWindow("commands")
	if parent == 0 {
		t.Fatal("parent was closed with child")
	}
	procSendMessage.Call(parent, 0x0010, 0, 0)
	select {
	case <-manager.done:
	case <-time.After(10 * time.Second):
		t.Fatal("parent did not close independently")
	}
}

func TestNativeDetachedWindows(t *testing.T) {
	if os.Getenv("LOG_AGENT_NATIVE_WINDOWS_TEST") != "1" {
		t.Skip("opt-in WebView2 integration test")
	}
	parentReady, childReady, releaseChild := make(chan struct{}, 1), make(chan struct{}, 1), make(chan struct{})
	errors := make(chan string, 2)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/parent":
			parentReady <- struct{}{}
			fmt.Fprint(w, `{}`)
		case "/api/child":
			childReady <- struct{}{}
			fmt.Fprint(w, `{}`)
		case "/api/hold":
			select {
			case <-releaseChild:
			case <-r.Context().Done():
			}
			fmt.Fprint(w, `{}`)
		case "/api/error":
			errors <- r.URL.Query().Get("message")
			fmt.Fprint(w, `{}`)
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			script := `await window.__lbOpenView('redis',-20000,-20000);await fetch('/api/parent');await window.__lbQuit();`
			if r.URL.Query().Get("detached") == "redis" {
				script = `await fetch('/api/child');await fetch('/api/hold');await window.__lbQuit();`
			}
			fmt.Fprintf(w, `<!doctype html><script>addEventListener('DOMContentLoaded',async()=>{try{%s}catch(e){await fetch('/api/error?message='+encodeURIComponent(String(e)));window.__lbQuit();}})</script>`, script)
		default:
			http.NotFound(w, r)
		}
	})
	dataPath := filepath.Join(t.TempDir(), "webview2")
	// WebView2's browser process exits asynchronously after the last controller.
	t.Cleanup(func() {
		deadline := time.Now().Add(8 * time.Second)
		for {
			err := os.RemoveAll(dataPath)
			if err == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Errorf("release WebView2 profile: %v", err)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
	manager := &desktopWindows{handler: handler, dataPath: dataPath, done: make(chan struct{})}
	if err := manager.open("database", -20000, -20000); err != nil {
		t.Fatal(err)
	}
	defer close(releaseChild)
	for _, ready := range []chan struct{}{parentReady, childReady} {
		select {
		case <-ready:
		case err := <-errors:
			t.Fatal(err)
		case <-time.After(25 * time.Second):
			t.Fatal("native window did not initialize")
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		manager.mu.Lock()
		active := manager.active
		manager.mu.Unlock()
		if active == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("closing source window did not leave child window alive")
		}
		time.Sleep(20 * time.Millisecond)
	}
	releaseChild <- struct{}{}
	select {
	case <-manager.done:
	case <-time.After(10 * time.Second):
		t.Fatal("last window did not close")
	}
	if err := manager.open("invalid", 0, 0); err == nil {
		t.Fatal("unknown view accepted")
	}
	if err := manager.open("redis", 0, 0); err == nil {
		t.Fatal("closed window manager accepted new window")
	}
}
