package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestTermOpenCancellationInterruptsHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	body, _ := json.Marshal(commandConnectionRequest{Host: host, Port: port, User: "tester", Auth: "password"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		(&server{}).handleTermOpen(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/term/open", bytes.NewReader(body)).WithContext(ctx))
	}()
	var conn net.Conn
	select {
	case conn = <-accepted:
	case <-time.After(3 * time.Second):
		t.Fatal("SSH connection was not attempted")
	}
	defer conn.Close()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelled handshake did not stop promptly")
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := io.Copy(io.Discard, conn); err != nil {
		t.Fatalf("cancelled SSH socket was not closed: %v", err)
	}
}

func TestE2ETerminalCancelConnection(t *testing.T) {
	browser := firstExistingPath(`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`)
	if browser == "" {
		t.Skip("Chrome or Edge required")
	}
	databaseTestSetup(t)
	web := httptest.NewServer(newHTTPHandler(newServer()))
	defer web.Close()
	allocator, stop := chromedp.NewExecAllocator(context.Background(), append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(browser), chromedp.NoSandbox, chromedp.WindowSize(1440, 950))...)
	defer stop()
	ctx, closeBrowser := chromedp.NewContext(allocator)
	defer closeBrowser()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	run := func(actions ...chromedp.Action) {
		t.Helper()
		if err := chromedp.Run(ctx, actions...); err != nil {
			t.Fatal(err)
		}
	}
	run(chromedp.Navigate(web.URL), chromedp.WaitVisible(`.rail .nav-item[data-view="terminal"]`, chromedp.ByQuery), chromedp.Click(`.rail .nav-item[data-view="terminal"]`, chromedp.ByQuery))
	run(chromedp.Evaluate(`(() => {
  state.commandServers=[{id:'cancel-test',name:'Cancel test',host:'test.invalid',user:'tester',auth:'password'}];
  renderTerminalView();
  window.__attempts=[];window.__closes=[];window.__cancelToasts=[];
  showToast=message=>window.__cancelToasts.push(message);
  const nativeFetch=window.fetch;
  window.fetch=(url,init={})=>{
   if(url==='/api/term/open')return new Promise((resolve,reject)=>{
    const attempt={resolve,reject,signal:init.signal};window.__attempts.push(attempt);
    if(window.__attempts.length===1)init.signal.addEventListener('abort',()=>reject(new DOMException('cancelled','AbortError')),{once:true});
   });
   if(String(url).startsWith('/api/term/close?')){window.__closes.push(url);return Promise.resolve(new Response('{}'));}
   return nativeFetch(url,init);
  };
 })()`, nil))
	run(chromedp.Click(`#term-connect`, chromedp.ByQuery), chromedp.WaitVisible(`#term-cancel-connect`, chromedp.ByQuery), chromedp.Click(`#term-cancel-connect`, chromedp.ByQuery), chromedp.Poll(`window.__attempts[0].signal.aborted && !document.querySelector('#term-connect').disabled && document.querySelectorAll('.term-tab').length===0 && window.__cancelToasts.length===0`, nil))
	// Simulate a late response even when the transport ignores cancellation.
	run(chromedp.Click(`#term-connect`, chromedp.ByQuery), chromedp.WaitVisible(`#term-cancel-connect`, chromedp.ByQuery), chromedp.Click(`#term-cancel-connect`, chromedp.ByQuery), chromedp.Click(`#term-connect`, chromedp.ByQuery), chromedp.Evaluate(`window.__attempts[1].resolve(new Response(JSON.stringify({id:'late-session'}),{status:200}))`, nil), chromedp.Poll(`window.__closes.some(url=>url.includes('late-session')) && document.querySelector('#term-connect').disabled && !document.querySelector('#term-cancel-connect').classList.contains('hidden') && document.querySelectorAll('.term-tab').length===1`, nil))
	// Closing a connecting tab must use the same cancellation path.
	run(chromedp.Click(`.term-tab-close`, chromedp.ByQuery), chromedp.Poll(`window.__attempts[2].signal.aborted && !document.querySelector('#term-connect').disabled && document.querySelector('#term-cancel-connect').classList.contains('hidden') && document.querySelectorAll('.term-tab').length===0`, nil), chromedp.Evaluate(`window.__attempts[2].reject(new Error('late failure'))`, nil), chromedp.Poll(`window.__cancelToasts.length===0`, nil))
}
