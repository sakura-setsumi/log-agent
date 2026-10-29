package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

func TestRedisProfileOrdering(t *testing.T) {
	redisTestSetup(t)
	original := []redisProfile{
		{ID: "a", Name: "A", Host: "localhost", Port: 6379, Password: "secret-a"},
		{ID: "b", Name: "B", Host: "localhost", Port: 6380, Password: "secret-b", TLS: true},
		{ID: "c", Name: "C", Host: "localhost", Port: 6381, User: "acl"},
	}
	if err := writeJSONFile(redisProfilesPath(), original); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		order []string
		want  []string
	}{
		{[]string{"c", "a", "b"}, []string{"c", "a", "b"}},
		{[]string{"b", "missing", "b"}, []string{"b", "c", "a"}},
		{[]string{}, []string{"b", "c", "a"}},
	} {
		response := redisRequest(t, http.MethodPut, "profiles", nil, map[string]any{"order": tc.order})
		if response.Code != 200 {
			t.Fatalf("reorder: %d %s", response.Code, response.Body)
		}
		stored, err := readRedisProfiles()
		if err != nil {
			t.Fatal(err)
		}
		if len(stored) != len(original) {
			t.Fatalf("lost profiles: %+v", stored)
		}
		for i, id := range tc.want {
			if stored[i].ID != id {
				t.Fatalf("order: %+v, want %v", stored, tc.want)
			}
			for _, before := range original {
				if before.ID == id && stored[i] != before {
					t.Fatalf("changed credentials: %s", id)
				}
			}
		}
	}
	response := redisRequest(t, http.MethodPut, "profiles", nil, map[string]any{"order": "invalid"})
	if response.Code != 400 {
		t.Fatalf("invalid order: %d", response.Code)
	}
}

func TestE2EConnectionManagerOrdering(t *testing.T) {
	browser := firstExistingPath(`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`)
	if browser == "" {
		t.Skip("Chrome or Edge required")
	}
	redisTestSetup(t)
	profiles, err := readRedisProfiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"second", "third"} {
		p := profiles[0]
		p.ID = id
		p.Name = id
		profiles = append(profiles, p)
	}
	if err := writeJSONFile(redisProfilesPath(), profiles); err != nil {
		t.Fatal(err)
	}
	dbProfiles := []databaseProfile{{ID: "a", Name: "Alpha", Engine: "mysql", Host: "localhost", User: "match"}, {ID: "b", Name: "Beta", Engine: "mysql", Host: "localhost", User: "hidden"}, {ID: "c", Name: "Gamma", Engine: "mysql", Host: "localhost", User: "match"}}
	if err := writeJSONFile(databaseProfilesPath(), dbProfiles); err != nil {
		t.Fatal(err)
	}
	web := httptest.NewServer(newHTTPHandler(newServer()))
	defer web.Close()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(browser), chromedp.NoSandbox, chromedp.WindowSize(1200, 900))
	allocator, closeAllocator := chromedp.NewExecAllocator(context.Background(), opts...)
	defer closeAllocator()
	ctx, closeBrowser := chromedp.NewContext(allocator, chromedp.WithErrorf(func(string, ...any) {}))
	defer closeBrowser()
	ctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
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
	// Send drag events with coordinates on each half of the target, through the actual DOM handlers.
	drag := func(source, target string, after bool) {
		t.Helper()
		fraction := "0.25"
		if after {
			fraction = "0.75"
		}
		run(chromedp.Evaluate(`(()=>{const source=document.querySelector('`+source+`'),target=document.querySelector('`+target+`'),rect=target.getBoundingClientRect(),dataTransfer=new DataTransfer();for(const [node,type] of [[source,'dragstart'],[target,'dragover'],[target,'drop'],[source,'dragend']])node.dispatchEvent(new DragEvent(type,{bubbles:true,cancelable:true,dataTransfer,clientY:rect.top+rect.height*`+fraction+`}));})()`, nil))
	}
	run(chromedp.Navigate(web.URL), chromedp.WaitVisible(`#menubar`, chromedp.ByQuery), chromedp.Evaluate(`openDatabaseManager()`, nil))
	wait(`document.querySelectorAll('[data-db-manager-edit]').length===3`)
	// Reorder while filtered; hidden connections remain in the full ordered list.
	run(chromedp.Evaluate(`document.querySelector('#db-manager-search').value='match';document.querySelector('#db-manager-search').dispatchEvent(new Event('input'))`, nil))
	drag(`[data-db-manager-edit="c"]`, `[data-db-manager-edit="a"]`, false)
	wait(`!document.querySelector('#db-manager-list').hasAttribute('aria-busy') && document.querySelector('[data-db-manager-edit]').dataset.dbManagerEdit==='c'`)
	storedDB, err := readDatabaseProfiles()
	if err != nil || storedDB[0].ID != "c" || storedDB[1].ID != "a" || storedDB[2].ID != "b" {
		t.Fatalf("database ordering: %+v %v", storedDB, err)
	}
	run(chromedp.Evaluate(`document.querySelector('[data-db-close="manager"]').click();document.querySelector('#redis-manage').click()`, nil))
	wait(`document.querySelector('#redis-manager-dialog').open && document.querySelectorAll('.redis-manager-row').length===3 && !document.querySelector('#redis-profile').disabled`)
	drag(`[data-redis-profile="test"]`, `[data-redis-profile="third"]`, true)
	wait(`!document.querySelector('#redis-manager-list').hasAttribute('aria-busy') && document.querySelector('#redis-profile option').value==='second'`)
	storedRedis, err := readRedisProfiles()
	if err != nil || storedRedis[0].ID != "second" || storedRedis[2].ID != "test" {
		t.Fatalf("Redis ordering: %+v %v", storedRedis, err)
	}
	// Failed persistence restores both the manager and selector.
	run(chromedp.Evaluate(`window.__realFetch=window.fetch;window.fetch=(url,options)=>String(url).startsWith('/api/redis/profiles')&&options?.method==='PUT'?Promise.resolve(new Response(JSON.stringify({detail:'fixture save failed'}),{status:500})):window.__realFetch(url,options)`, nil))
	drag(`[data-redis-profile="test"]`, `[data-redis-profile="second"]`, false)
	wait(`!document.querySelector('#redis-manager-list').hasAttribute('aria-busy') && document.querySelector('#redis-profile option').value==='second' && document.querySelector('#redis-status').textContent==='fixture save failed'`)
	run(chromedp.Evaluate(`window.fetch=window.__realFetch;document.querySelector('#redis-close-manager').click();state.commandServers=[{id:'s1',name:'One',host:'localhost'},{id:'s2',name:'Two',host:'localhost'},{id:'s3',name:'Three',host:'localhost'}];openServerManager('s1')`, nil))
	wait(`document.querySelectorAll('[data-manage-server-id]').length===3`)
	// Exercise native mouse dragging from the visible handle for the server list.
	var sourcePoint, targetPoint struct{ X, Y float64 }
	run(chromedp.Evaluate(`(()=>{const r=document.querySelector('[data-manage-server-id="s3"] .connection-order-handle').getBoundingClientRect();return {X:r.left+r.width/2,Y:r.top+r.height/2}})()`, &sourcePoint),
		chromedp.Evaluate(`(()=>{const r=document.querySelector('[data-manage-server-id="s1"]').getBoundingClientRect();return {X:r.left+r.width/2,Y:r.top+r.height/4}})()`, &targetPoint))
	run(chromedp.ActionFunc(func(ctx context.Context) error {
		if err := input.DispatchMouseEvent(input.MouseMoved, sourcePoint.X, sourcePoint.Y).Do(ctx); err != nil {
			return err
		}
		if err := input.DispatchMouseEvent(input.MousePressed, sourcePoint.X, sourcePoint.Y).WithButton(input.Left).WithClickCount(1).Do(ctx); err != nil {
			return err
		}
		for step := 1; step <= 12; step++ {
			f := float64(step) / 12
			if err := input.DispatchMouseEvent(input.MouseMoved, sourcePoint.X+(targetPoint.X-sourcePoint.X)*f, sourcePoint.Y+(targetPoint.Y-sourcePoint.Y)*f).WithButton(input.Left).WithButtons(1).Do(ctx); err != nil {
				return err
			}
		}
		return input.DispatchMouseEvent(input.MouseReleased, targetPoint.X, targetPoint.Y).WithButton(input.Left).WithClickCount(1).Do(ctx)
	}))
	wait(`document.querySelector('[data-manage-server-id]').dataset.manageServerId==='s3' && state.managedCommandServerId==='s1' && !document.querySelector('#server-manager-list').hasAttribute('aria-busy')`)
	wait(`fetch('/api/commands/state').then(response=>response.json()).then(payload=>payload.commands?.servers?.[0]?.id==='s3')`)
	run(chromedp.Reload(), chromedp.WaitVisible(`#menubar`, chromedp.ByQuery), chromedp.Evaluate(`openServerManager()`, nil))
	wait(`document.querySelector('[data-manage-server-id]')?.dataset.manageServerId==='s3'`)
	run(chromedp.Evaluate(`closeServerManager();openDatabaseManager()`, nil))
	wait(`document.querySelector('[data-db-manager-edit]')?.dataset.dbManagerEdit==='c'`)
	run(chromedp.Evaluate(`document.querySelector('[data-db-close="manager"]').click();document.querySelector('#redis-manage').click()`, nil))
	wait(`document.querySelector('#redis-manager-dialog').open && document.querySelector('.redis-manager-row')?.dataset.redisProfile==='second'`)
}
