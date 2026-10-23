package main

import (
	"context"
	"encoding/json"
	"github.com/chromedp/chromedp"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestE2EInsertTemplateValues(t *testing.T) {
	browser := firstExistingPath(`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`)
	if browser == "" {
		t.Skip("Chrome or Edge required")
	}
	a, _ := databaseTestSetup(t)
	web := httptest.NewServer(newHTTPHandler(newServer()))
	defer web.Close()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(browser), chromedp.NoSandbox, chromedp.WindowSize(1400, 1000))
	allocator, cancelAllocator := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAllocator()
	ctx, cancelBrowser := chromedp.NewContext(allocator, chromedp.WithErrorf(func(string, ...any) {}))
	defer cancelBrowser()
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
		if err := chromedp.Run(ctx, chromedp.Poll(expression, nil, chromedp.WithPollingTimeout(8*time.Second))); err != nil {
			var dump string
			_ = chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#db-sql-editor').innerText+' | '+document.querySelector('#db-inline-picker').innerText`, &dump))
			t.Fatalf("%s: %v; %s", expression, err, dump)
		}
	}
	check := func(expression string) {
		t.Helper()
		var ok bool
		run(chromedp.Evaluate(expression, &ok))
		if !ok {
			t.Fatal(expression)
		}
	}
	click := func(selector string) {
		t.Helper()
		quoted, _ := json.Marshal(selector)
		run(chromedp.Evaluate("document.querySelector("+string(quoted)+").click()", nil))
	}
	hasSQL := func(sql string) {
		t.Helper()
		quoted, _ := json.Marshal(sql)
		wait("document.querySelector('#db-sql-editor .cm-content').textContent.includes(" + string(quoted) + ")")
	}
	run(chromedp.Navigate(web.URL), chromedp.WaitVisible(`.nav-item[data-view="database"]`, chromedp.ByQuery))
	click(`.nav-item[data-view="database"]`)
	wait(`!!document.querySelector('[data-db-connect="east"]')`)
	click(`#db-target-select`)
	click(`[data-db-target-group="east"]`)
	wait(`document.querySelector('#db-target-select').dataset.count==='1'`)
	click(`#db-target-picker-close`)
	click(`#db-toggle-templates`)
	click(`[data-db-template="insert"]`)
	click(`[data-db-slot-kind="table"]`)
	wait(`!!document.querySelector('#db-inline-table') && !document.querySelector('#db-inline-apply').disabled`)
	click(`#db-inline-apply`)
	click(`[data-db-slot-kind="columns"]`)
	wait(`document.querySelectorAll('#db-inline-body [name="inline-column"]').length===3`)
	check(`!document.querySelector('#db-inline-table') && !document.querySelector('#db-inline-source')`)
	click(`#db-inline-none`)
	click(`#db-inline-body [value="id"]`)
	click(`#db-inline-body [value="note"]`)
	click(`#db-inline-apply`)
	hasSQL(`("id", "note") VALUES (NULL, NULL)`)
	click(`[data-db-slot-kind="values"]`)
	wait(`!!document.querySelector('#db-insert-bulk')`)
	check(`!document.querySelector('#db-inline-table') && document.querySelectorAll('[data-insert-row]').length===1`)
	bulk, _ := json.Marshal("1\tO'Brien\r\n2\tNULL\r\n3\t\"NULL\"")
	run(chromedp.Evaluate("document.querySelector('#db-insert-bulk').value="+string(bulk), nil))
	click(`#db-insert-import`)
	wait(`document.querySelectorAll('[data-insert-row]').length===3`)
	if path := os.Getenv("LOG_AGENT_INSERT_PREVIEW"); path != "" {
		var screenshot []byte
		run(chromedp.CaptureScreenshot(&screenshot))
		if err := os.WriteFile(path, screenshot, 0600); err != nil {
			t.Fatal(err)
		}
	}
	click(`#db-inline-apply`)
	hasSQL(`VALUES (1, 'O''Brien'), (2, NULL), (3, 'NULL');`)
	click(`[data-db-slot-kind="columns"]`)
	wait(`!!document.querySelector('#db-inline-body [value="amount"]')`)
	check(`!document.querySelector('#db-inline-table')`)
	click(`#db-inline-body [value="amount"]`)
	click(`#db-inline-apply`)
	hasSQL(`VALUES (1, NULL, 'O''Brien'), (2, NULL, NULL), (3, NULL, 'NULL');`)
	click(`[data-db-slot-kind="values"]`)
	wait(`document.querySelectorAll('[data-insert-row]').length===3`)
	click(`#db-insert-add`)
	wait(`document.querySelectorAll('[data-insert-row]').length===4`)
	click(`[data-insert-remove="3"]`)
	wait(`document.querySelectorAll('[data-insert-row]').length===3`)
	click(`#db-inline-close`)
	for _, item := range []struct{ id, slot string }{{"select", "columns"}, {"update", "field"}, {"delete", "condition"}} {
		click(`[data-db-template="` + item.id + `"]`)
		click(`[data-db-slot-kind="` + item.slot + `"]`)
		wait(`!!document.querySelector('#db-inline-body [name="inline-column"]')`)
		check(`!document.querySelector('#db-inline-table') && !document.querySelector('#db-inline-source')`)
		click(`#db-inline-close`)
	}
	click(`[data-db-template="delete"]`)
	click(`[data-db-slot-kind="condition"]`)
	wait(`!!document.querySelector('#db-inline-body [name="inline-column"]')`)
	click(`#db-inline-apply`)
	wait(`!!document.querySelector('#db-inline-operator')`)
	run(chromedp.Evaluate(`document.querySelector('#db-inline-operator').value='IN';document.querySelector('#db-inline-operator').dispatchEvent(new Event('change'));document.querySelector('#db-inline-list').value='1\n2\n3'`, nil))
	click(`#db-inline-apply`)
	hasSQL(`IN (1, 2, 3)`)
	pool, _ := databases.pool(context.Background(), a)
	var count int
	if err := pool.db.QueryRow("SELECT COUNT(*) FROM orders").Scan(&count); err != nil || count != 2 {
		t.Fatalf("template editing wrote to database: %d %v", count, err)
	}
}
