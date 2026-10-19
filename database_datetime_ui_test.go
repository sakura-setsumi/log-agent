package main

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestE2EDatabaseDateTimePicker(t *testing.T) {
	browser := firstExistingPath(`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`)
	if browser == "" {
		t.Skip("Chrome or Edge required")
	}
	target, _ := databaseTestSetup(t)
	pool, err := databases.pool(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.db.Exec(`CREATE TABLE appointments(id INTEGER PRIMARY KEY, expires DATETIME, day DATE, clock TIME); INSERT INTO appointments VALUES(1, '2026-09-18T11:07:26Z', '2026-09-18', '11:07:26')`)
	if err != nil {
		t.Fatal(err)
	}
	web := httptest.NewServer(newHTTPHandler(newServer()))
	defer web.Close()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(browser), chromedp.NoSandbox, chromedp.WindowSize(1600, 1000))
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
		run(chromedp.Poll(expression, nil, chromedp.WithPollingTimeout(10*time.Second)))
	}
	run(chromedp.Navigate(web.URL), chromedp.WaitVisible(`.nav-item[data-view="database"]`, chromedp.ByQuery), chromedp.Click(`.nav-item[data-view="database"]`, chromedp.ByQuery))
	wait(`!!document.querySelector('[data-db-connect="east"]')`)
	run(chromedp.Click(`[data-db-connect="east"]`, chromedp.ByQuery))
	wait(`!!document.querySelector('[data-db-database]')`)
	run(chromedp.Click(`[data-db-database='["east","main"]']`, chromedp.ByQuery))
	wait(`Array.from(document.querySelectorAll('[data-db-table]')).some(b=>b.textContent.includes('appointments'))`)
	run(chromedp.Evaluate(`Array.from(document.querySelectorAll('[data-db-table]')).find(b=>b.textContent.includes('appointments')).click()`, nil))
	wait(`document.querySelectorAll('#db-data-grid .tabulator-row').length===1`)
	// Record calls while letting the browser open its native picker.
	run(chromedp.Evaluate(`window.pickerCalls=0;const nativePicker=HTMLInputElement.prototype.showPicker;HTMLInputElement.prototype.showPicker=function(){window.pickerCalls++;return nativePicker.call(this)}`, nil))
	run(chromedp.Click(`#db-data-grid .tabulator-row [tabulator-field="v1"]`, chromedp.ByQuery))
	wait(`window.pickerCalls===1 && document.querySelector('#db-data-grid input[type="datetime-local"]').value==='2026-09-18T11:07:26'`)
	// Choosing a date must leave the editor alive so time can also be adjusted.
	run(chromedp.Evaluate(`(() => {const i=document.querySelector('#db-data-grid input[type="datetime-local"]');i.value='2026-09-20T15:30:42';i.dispatchEvent(new Event('change',{bubbles:true}));})()`, nil))
	wait(`!!document.querySelector('#db-data-grid input[type="datetime-local"]') && document.querySelector('#db-save-rows').disabled`)
	run(chromedp.Evaluate(`document.querySelector('#db-data-grid input[type="datetime-local"]').dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true}))`, nil))
	wait(`!document.querySelector('#db-save-rows').disabled && document.querySelector('#db-data-grid [tabulator-field="v1"].db-cell-dirty').textContent==='2026-09-20T15:30:42Z'`)
	run(chromedp.Click(`#db-save-rows`, chromedp.ByQuery))
	wait(`document.querySelector('#db-status').textContent.includes('Committed 1')`)
	var saved string
	if err := pool.db.QueryRow(`SELECT CAST(expires AS TEXT) FROM appointments WHERE id=1`).Scan(&saved); err != nil || saved != "2026-09-20T15:30:42Z" {
		t.Fatalf("saved datetime %q: %v", saved, err)
	}
	for _, entry := range []struct{ field, kind, value string }{{"v2", "date", "2026-09-18"}, {"v3", "time", "11:07:26"}} {
		run(chromedp.Click(`#db-data-grid .tabulator-row [tabulator-field="`+entry.field+`"]`, chromedp.ByQuery))
		wait(`document.querySelector('#db-data-grid input[type="` + entry.kind + `"]').value==='` + entry.value + `'`)
		run(chromedp.Evaluate(`document.querySelector('#db-data-grid .tabulator-editing input').dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}))`, nil))
		wait(`!document.querySelector('#db-data-grid .tabulator-editing') && document.querySelector('#db-save-rows').disabled`)
	}
}
