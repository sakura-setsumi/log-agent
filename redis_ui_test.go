package main

import (
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

func TestE2ERedisWorkspace(t *testing.T) {
	browser := firstExistingPath(`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`)
	if browser == "" {
		t.Skip("Chrome or Edge required")
	}
	_, c := redisTestSetup(t)
	c.Set(context.Background(), "biz:wx:accessToken", `{"token":"demo","active":true}`, time.Minute)
	c.HSet(context.Background(), "biz:wx:member:plans", "vip", "365", "svip", "730")
	c.RPush(context.Background(), "agent:jobs", "job-1", "job-2")
	c.SAdd(context.Background(), "sys:tags", "production", "cache")
	web := httptest.NewServer(newHTTPHandler(newServer()))
	defer web.Close()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(browser), chromedp.NoSandbox, chromedp.WindowSize(1450, 950))
	allocator, closeAllocator := chromedp.NewExecAllocator(context.Background(), opts...)
	defer closeAllocator()
	ctx, closeBrowser := chromedp.NewContext(allocator, chromedp.WithErrorf(func(string, ...any) {}))
	defer closeBrowser()
	ctx, cancel := context.WithTimeout(ctx, 80*time.Second)
	defer cancel()
	var exceptions []string
	chromedp.ListenTarget(ctx, func(event any) {
		if e, ok := event.(*runtime.EventExceptionThrown); ok {
			exceptions = append(exceptions, e.ExceptionDetails.Text)
		}
	})
	run := func(actions ...chromedp.Action) {
		t.Helper()
		if err := chromedp.Run(ctx, actions...); err != nil {
			t.Fatal(err)
		}
	}
	wait := func(expression string) {
		t.Helper()
		run(chromedp.Poll(expression, nil, chromedp.WithPollingTimeout(10*time.Second)))
	}
	run(chromedp.Navigate(web.URL), chromedp.WaitReady(`.nav-item[data-view="redis"]`, chromedp.ByQuery), chromedp.Evaluate(`setLanguage('zh')`, nil), chromedp.Click(`#menubar .menu-title[data-i18n="nav.redis"]`, chromedp.ByQuery), chromedp.Click(`#redis-add`, chromedp.ByQuery))
	wait(`document.querySelector('#redis-connection-dialog').open`)
	run(chromedp.Click(`#redis-close-dialog`, chromedp.ByQuery))
	wait(`document.querySelectorAll('#redis-profile option').length===2 && !document.querySelector('#redis-profile').disabled`)
	run(chromedp.Evaluate(`document.querySelector('#redis-profile').value='test';document.querySelector('#redis-profile').dispatchEvent(new Event('change'))`, nil))
	wait(`document.querySelectorAll('[data-redis-key]').length===4 && !document.querySelector('#redis-profile').disabled`)
	var expandedGroups int
	run(chromedp.Evaluate(`document.querySelectorAll('#redis-tree details[open]').length`, &expandedGroups))
	if expandedGroups != 0 {
		t.Fatal("Redis groups should default to collapsed")
	}
	run(chromedp.Evaluate(`for(let node=document.querySelector('[data-redis-key="biz:wx:accessToken"]');node;node=node.parentElement){if(node.tagName==='DETAILS')node.open=true;}`, nil))
	wait(`document.querySelectorAll('#redis-tree details[open]').length===2`)
	run(chromedp.Click(`[data-redis-key="biz:wx:accessToken"]`, chromedp.ByQuery))
	wait(`document.querySelector('#redis-editor .cm-content') && !document.querySelector('#redis-save').disabled`)
	var layout string
	run(chromedp.Evaluate(`JSON.stringify({redis:!document.querySelector('#redis-view').classList.contains('hidden'),databaseHidden:document.querySelector('#database-view').classList.contains('hidden'),width:document.querySelector('#redis-editor').getBoundingClientRect().width,height:document.querySelector('#redis-editor').getBoundingClientRect().height,overflow:document.documentElement.scrollWidth>innerWidth})`, &layout))
	if !strings.Contains(layout, `"redis":true`) || !strings.Contains(layout, `"databaseHidden":true`) || strings.Contains(layout, `"overflow":true`) {
		t.Fatalf("bad layout: %s", layout)
	}
	run(chromedp.Click(`#redis-pretty`, chromedp.ByQuery))
	wait(`document.querySelector('#redis-dirty').textContent!==''`)
	run(chromedp.Click(`#redis-save`, chromedp.ByQuery))
	wait(`document.querySelector('#redis-status').textContent==='已保存' && document.querySelector('#redis-dirty').textContent===''`)
	if !strings.Contains(c.Get(context.Background(), "biz:wx:accessToken").Val(), "\n") {
		t.Fatal("formatted value was not saved")
	}
	if c.TTL(context.Background(), "biz:wx:accessToken").Val() <= 0 {
		t.Fatal("save lost TTL")
	}
	run(chromedp.Evaluate(`document.querySelector('#redis-ttl').value='120';document.querySelector('#redis-ttl').dispatchEvent(new Event('input'))`, nil), chromedp.Click(`#redis-expire`, chromedp.ByQuery))
	wait(`document.querySelector('#redis-current-ttl').textContent.includes('120') && !document.querySelector('#redis-save').disabled`)
	run(chromedp.Click(`#redis-new-key`, chromedp.ByQuery), chromedp.Evaluate(`document.querySelector('#redis-key').value='new:key';document.querySelector('#redis-key').dispatchEvent(new Event('input'))`, nil), chromedp.Focus(`#redis-editor .cm-content`, chromedp.ByQuery), chromedp.ActionFunc(func(ctx context.Context) error { return input.InsertText("新值").Do(ctx) }), chromedp.Click(`#redis-save`, chromedp.ByQuery))
	wait(`document.querySelector('#redis-status').textContent==='已保存' && document.querySelector('#redis-key').value==='new:key'`)
	if c.Get(context.Background(), "new:key").Val() != "新值" {
		t.Fatal("new key not persisted")
	}
	run(chromedp.Evaluate(`for(let node=document.querySelector('[data-redis-key="new:key"]');node;node=node.parentElement){if(node.tagName==='DETAILS')node.open=true;}window.__redisClipboard=null;Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async text=>{window.__redisClipboard=text;}}})`, nil), chromedp.Click(`[data-redis-action="copy"][data-key="new:key"]`, chromedp.ByQuery))
	wait(`window.__redisClipboard==='新值' && !document.querySelector('#redis-profile').disabled`)
	run(chromedp.Evaluate(`window.__redisClipboard=null;document.body.dispatchEvent(new KeyboardEvent('keydown',{key:'c',ctrlKey:true,bubbles:true,cancelable:true}))`, nil))
	wait(`window.__redisClipboard==='新值' && !document.querySelector('#redis-profile').disabled`)
	run(chromedp.Focus(`#redis-editor .cm-content`, chromedp.ByQuery), chromedp.Evaluate(`window.__redisClipboard=null;document.querySelector('#redis-editor .cm-content').dispatchEvent(new KeyboardEvent('keydown',{key:'c',ctrlKey:true,bubbles:true,cancelable:true}))`, nil))
	var editorCopyIntercepted bool
	run(chromedp.Evaluate(`window.__redisClipboard!==null`, &editorCopyIntercepted))
	if editorCopyIntercepted {
		t.Fatal("key shortcut intercepted editor copy")
	}
	run(chromedp.Evaluate(`window.__redisOriginalConfirm=window.confirm;window.confirm=message=>{window.__redisConfirm=message;return false;};document.querySelector('[data-redis-action="delete"][data-key="new:key"]').click()`, nil))
	if c.Exists(context.Background(), "new:key").Val() != 1 {
		t.Fatal("cancelled deletion removed key")
	}
	run(chromedp.Evaluate(`window.confirm=()=>true;document.querySelector('[data-redis-action="delete"][data-key="new:key"]').click()`, nil))
	wait(`document.querySelector('#redis-status').textContent==='键已删除' && !document.querySelector('[data-redis-key="new:key"]')`)
	if c.Exists(context.Background(), "new:key").Val() != 0 {
		t.Fatal("row deletion did not remove key")
	}
	run(chromedp.Evaluate(`window.confirm=window.__redisOriginalConfirm`, nil))
	run(chromedp.Evaluate(`document.querySelector('#redis-pattern').value='biz:*'`, nil), chromedp.Click(`#redis-search-form button`, chromedp.ByQuery))
	wait(`document.querySelectorAll('[data-redis-key]').length===2 && !document.querySelector('#redis-profile').disabled`)
	run(chromedp.Evaluate(`document.querySelectorAll('#redis-tree details[open]').length`, &expandedGroups))
	if expandedGroups != 0 {
		t.Fatal("new search should start with collapsed groups")
	}
	run(chromedp.Evaluate(`for(let node=document.querySelector('[data-redis-key="biz:wx:member:plans"]');node;node=node.parentElement){if(node.tagName==='DETAILS')node.open=true;}`, nil))
	run(chromedp.Click(`[data-redis-key="biz:wx:member:plans"]`, chromedp.ByQuery))
	wait(`document.querySelector('#redis-type').value==='hash' && !document.querySelector('#redis-save').disabled`)
	run(chromedp.Evaluate(`window.__redisClipboard=null;document.querySelector('[data-redis-action="copy"][data-key="biz:wx:accessToken"]').click()`, nil))
	wait(`window.__redisClipboard?.includes('"token"') && !document.querySelector('#redis-profile').disabled`)
	run(chromedp.Evaluate(`window.confirm=()=>true;document.querySelector('[data-redis-action="delete"][data-key="biz:wx:accessToken"]').click()`, nil))
	wait(`document.querySelector('#redis-status').textContent==='键已删除' && !document.querySelector('[data-redis-key="biz:wx:accessToken"]') && document.querySelector('#redis-key').value==='biz:wx:member:plans' && document.querySelector('#redis-type').value==='hash'`)
	run(chromedp.Evaluate(`window.confirm=window.__redisOriginalConfirm`, nil))
	run(chromedp.Click(`#menubar .menu-title[data-i18n="nav.redis"]`, chromedp.ByQuery), chromedp.Click(`#redis-manage`, chromedp.ByQuery))
	wait(`document.querySelector('#redis-manager-dialog').open && document.querySelector('[data-redis-manage="edit"]') && !document.querySelector('#redis-manage').disabled`)
	run(chromedp.Click(`[data-redis-manage="edit"][data-id="test"]`, chromedp.ByQuery))
	wait(`document.querySelector('#redis-connection-dialog').open`)
	run(chromedp.Click(`#redis-test`, chromedp.ByQuery))
	wait(`document.querySelector('#redis-connection-status').textContent==='连接成功'`)
	run(chromedp.KeyEvent(kb.Escape), chromedp.Evaluate(`setLanguage('en')`, nil))
	wait(`document.querySelector('#redis-add').textContent==='New connection'`)
	run(chromedp.Evaluate(`setLanguage('zh')`, nil))
	if os.Getenv("LOG_AGENT_REDIS_SCREENSHOT") != "" {
		run(chromedp.Click(`#menubar .menu-title[data-i18n="nav.redis"]`, chromedp.ByQuery))
		var png []byte
		run(chromedp.CaptureScreenshot(&png))
		if err := os.WriteFile("redis-workspace.png", png, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if len(exceptions) > 0 {
		t.Fatalf("JavaScript errors: %v", exceptions)
	}
}
