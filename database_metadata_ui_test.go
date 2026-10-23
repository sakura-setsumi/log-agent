package main

import (
	"context"
	"github.com/chromedp/chromedp"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestE2EDatabaseMetadataTabs(t *testing.T) {
	browser := firstExistingPath(`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`)
	if browser == "" {
		t.Skip("Chrome or Edge required")
	}
	target, _ := databaseTestSetup(t)
	pool, err := databases.pool(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{"CREATE TABLE meta_items (id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES orders(id), amount INTEGER CHECK(amount>0))", "CREATE INDEX meta_amount ON meta_items(amount)", "CREATE TRIGGER meta_insert AFTER INSERT ON meta_items BEGIN UPDATE meta_items SET amount=amount WHERE id=NEW.id; END"} {
		if _, err := pool.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	web := httptest.NewServer(newHTTPHandler(newServer()))
	defer web.Close()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(browser), chromedp.NoSandbox, chromedp.WindowSize(1500, 1000))
	allocator, closeAllocator := chromedp.NewExecAllocator(context.Background(), opts...)
	defer closeAllocator()
	ctx, closeBrowser := chromedp.NewContext(allocator, chromedp.WithErrorf(func(string, ...any) {}))
	defer closeBrowser()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	run := func(actions ...chromedp.Action) {
		t.Helper()
		if err := chromedp.Run(ctx, actions...); err != nil {
			t.Fatal(err)
		}
	}
	wait := func(expression string) {
		t.Helper()
		run(chromedp.Poll(expression, nil, chromedp.WithPollingTimeout(8*time.Second)))
	}
	run(chromedp.Navigate(web.URL), chromedp.WaitReady(`.nav-item[data-view="database"]`, chromedp.ByQuery), chromedp.Click(`.nav-item[data-view="database"]`, chromedp.ByQuery))
	wait(`!!document.querySelector('[data-db-connect="east"]')`)
	run(chromedp.Click(`[data-db-connect="east"]`, chromedp.ByQuery))
	wait(`!!document.querySelector('[data-db-database]')`)
	run(chromedp.Click(`[data-db-database='["east","main"]']`, chromedp.ByQuery))
	wait(`Array.from(document.querySelectorAll('[data-db-table]')).some(b=>b.textContent.includes('meta_items'))`)
	run(chromedp.Evaluate(`Array.from(document.querySelectorAll('[data-db-table]')).find(b=>b.textContent.includes('meta_items')).parentElement.querySelector('[data-db-table-toggle]').click()`, nil))
	wait(`document.querySelectorAll('[data-db-structure-open]').length===5`)
	for _, kind := range []string{"indexes", "foreignKeys", "checks", "triggers"} {
		run(chromedp.Click(`[data-db-structure-open][data-group="`+kind+`"]`, chromedp.ByQuery))
		wait(`document.querySelector('[data-db-structure-tab="` + kind + `"].active') && !document.querySelector('[data-db-structure-page="` + kind + `"]').classList.contains('hidden')`)
		wait(`document.querySelector('[data-db-structure-page="` + kind + `"] .tabulator-row')`)
	}
	var expanded bool
	run(chromedp.Evaluate(`document.querySelector('[data-db-structure-group][data-group="indexes"]').getAttribute('aria-expanded')==='true'`, &expanded))
	if expanded {
		t.Fatal("group label expanded tree")
	}
	if os.Getenv("LOG_AGENT_METADATA_SCREENSHOT") != "" {
		var png []byte
		run(chromedp.CaptureScreenshot(&png))
		if err := os.WriteFile("database-metadata-tabs.png", png, 0600); err != nil {
			t.Fatal(err)
		}
	}
	run(chromedp.Click(`[data-db-structure-tab="comment"]`, chromedp.ByQuery))
	wait(`document.querySelector('#db-table-comment').disabled`)
	// Exercise client preview/apply with isolated responses; actual MySQL SQL
	// generation is covered by TestDatabaseMySQLCommentPlan.
	run(chromedp.Evaluate(`window.__metaFetch=window.fetch;window.__commentBodies=[];window.fetch=async(url,options)=>{if(String(url).includes('/api/database/structure?')){const response=await window.__metaFetch(url,options),data=await response.json();return new Response(JSON.stringify({...data,commentSupported:true,comment:'old'}),{status:200});}if(String(url).includes('/api/database/schema?')){const body=JSON.parse(options.body);window.__commentBodies.push(body);return new Response(JSON.stringify(body.hash?{ok:true}:{sql:['ALTER TABLE meta_items COMMENT = ...'],hash:'preview-hash'}),{status:200});}return window.__metaFetch(url,options);};`, nil), chromedp.Click(`#db-meta-reload`, chromedp.ByQuery))
	wait(`!document.querySelector('#db-table-comment').disabled && document.querySelector('#db-table-comment').value==='old'`)
	run(chromedp.Evaluate(`const area=document.querySelector('#db-table-comment');area.value='中文表备注';area.dispatchEvent(new Event('input',{bubbles:true}));`, nil), chromedp.Click(`#db-comment-preview`, chromedp.ByQuery))
	wait(`!document.querySelector('#db-comment-save').disabled && document.querySelector('#db-comment-sql').textContent.includes('ALTER TABLE')`)
	run(chromedp.Click(`#db-comment-save`, chromedp.ByQuery))
	wait(`document.querySelector('#db-comment-save').disabled && window.__commentBodies.length===2`)
	var saved bool
	run(chromedp.Evaluate(`window.__commentBodies[1].action==='setComment' && window.__commentBodies[1].comment==='中文表备注' && window.__commentBodies[1].hash==='preview-hash'`, &saved))
	if !saved {
		t.Fatal("comment preview/apply payload mismatch")
	}
}
