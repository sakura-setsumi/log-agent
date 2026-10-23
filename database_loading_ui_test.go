package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

func TestE2EDatabaseLoading(t *testing.T) {
	browser := firstExistingPath(`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`)
	if browser == "" {
		t.Skip("Chrome or Edge required")
	}
	databaseTestSetup(t)
	web := httptest.NewServer(newHTTPHandler(newServer()))
	defer web.Close()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(browser), chromedp.NoSandbox, chromedp.WindowSize(1400, 1000))
	allocator, cancelAllocator := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAllocator()
	ctx, cancelBrowser := chromedp.NewContext(allocator, chromedp.WithErrorf(func(string, ...any) {}))
	defer cancelBrowser()
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
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
	typeText := func(text string) {
		t.Helper()
		run(chromedp.ActionFunc(func(ctx context.Context) error { return input.InsertText(text).Do(ctx) }))
	}

	click := func(selector string) {
		t.Helper()
		quoted, _ := json.Marshal(selector)
		run(chromedp.Evaluate("document.querySelector("+string(quoted)+").click()", nil))
	}
	setSQL := func(sql string) {
		t.Helper()
		run(chromedp.Focus(`#db-sql-editor .cm-content`, chromedp.ByQuery), chromedp.ActionFunc(func(ctx context.Context) error {
			if err := input.DispatchKeyEvent(input.KeyRawDown).WithKey("a").WithCode("KeyA").WithWindowsVirtualKeyCode(65).WithModifiers(input.ModifierCtrl).Do(ctx); err != nil {
				return err
			}
			return input.DispatchKeyEvent(input.KeyUp).WithKey("a").WithCode("KeyA").WithWindowsVirtualKeyCode(65).WithModifiers(input.ModifierCtrl).Do(ctx)
		}))
		typeText(sql)
	}

	run(chromedp.Navigate(web.URL), chromedp.WaitVisible(`.nav-item[data-view="database"]`, chromedp.ByQuery))
	click(`.nav-item[data-view="database"]`)
	wait(`!!document.querySelector('[data-db-connect="east"]')`)
	click(`#db-target-select`)
	click(`[data-db-target-group="east"]`)
	wait(`document.querySelector('#db-target-select').dataset.count==='1'`)
	click(`#db-target-picker-close`)
	// Hold real responses after the server reads them, so the UI polls real row counts.
	run(chromedp.Evaluate(`window.dbLoadFetch=window.fetch;window.dbLoadHold='query';window.dbLoadResponse=null;window.fetch=async(url,options)=>{
      const response=await window.dbLoadFetch(url,options);
      if(!String(url).includes('/api/database/'+window.dbLoadHold+'?'))return response;
      return new Promise((resolve,reject)=>{
        window.dbLoadResponse=()=>resolve(response);
        options.signal.addEventListener('abort',()=>reject(new DOMException('Aborted','AbortError')),{once:true});
        if(options.signal.aborted)reject(new DOMException('Aborted','AbortError'));
      });
    }`, nil))
	setSQL(`SELECT * FROM orders ORDER BY id;`)
	click(`#db-run-query`)
	wait(`!document.querySelector('#db-query-loading').classList.contains('hidden') && document.querySelector('#db-query-load-records').textContent==='2'`)
	check(`document.querySelector('#db-panel-result').getAttribute('aria-busy')==='true' && !document.querySelector('#db-query-load-stop').disabled`)
	wait(`parseFloat(document.querySelector('#db-query-load-time').textContent)>=0.3`)
	if path := os.Getenv("LOG_AGENT_LOADING_PREVIEW"); path != "" {
		var screenshot []byte
		run(chromedp.Evaluate(`setLanguage('zh')`, nil), chromedp.CaptureScreenshot(&screenshot))
		if err := os.WriteFile(path, screenshot, 0600); err != nil {
			t.Fatal(err)
		}
		run(chromedp.Evaluate(`setLanguage('en')`, nil))
	}
	// An unfinished request must not cover a different result tab.
	run(chromedp.Evaluate(`window.dbLoadOriginalTab=document.querySelector('#db-sql-tabs .active [data-db-sql]').dataset.dbSql;window.dbLoadHold='none'`, nil))
	click(`#db-add-sql-tab`)
	setSQL(`SELECT 42 AS other_result;`)
	click(`#db-run-query`)
	wait(`document.querySelector('#db-query-loading').classList.contains('hidden') && document.querySelector('#db-query-grid').textContent.includes('42')`)
	run(chromedp.Evaluate(`window.dbLoadResponse()`, nil))
	wait(`document.querySelector('#db-query-loading').classList.contains('hidden') && document.querySelector('#db-query-grid').textContent.includes('42')`)
	run(chromedp.Evaluate(`document.querySelector('[data-db-sql="'+window.dbLoadOriginalTab+'"]').click();window.dbLoadHold='query'`, nil))
	wait(`document.querySelector('#db-query-loading').classList.contains('hidden') && document.querySelectorAll('#db-query-grid .tabulator-row').length===2`)
	check(`document.querySelector('#db-panel-result').getAttribute('aria-busy')==='false'`)
	click(`#db-run-query`)
	wait(`!document.querySelector('#db-query-loading').classList.contains('hidden') && document.querySelector('#db-query-load-records').textContent==='2'`)
	click(`#db-query-load-stop`)
	wait(`document.querySelector('#db-query-loading').classList.contains('hidden') && !document.querySelector('#db-run-query').disabled`)
	check(`document.querySelector('#db-query-summary').textContent.toLowerCase().includes('cancelled')`)
	// Open a table: same fetching card, actual record count, and cancellable refresh.
	run(chromedp.Evaluate(`window.dbLoadHold='table'`, nil))
	click(`[data-db-connect="east"]`)
	wait(`!!document.querySelector('[data-db-database]')`)
	click(`[data-db-database]`)
	wait(`!!document.querySelector('[data-db-table]')`)
	click(`[data-db-table]`)
	wait(`!document.querySelector('#db-data-loading').classList.contains('hidden') && document.querySelector('#db-data-load-records').textContent==='2'`)
	check(`document.querySelector('#db-query-loading').classList.contains('hidden') && document.querySelector('#db-panel-data').getAttribute('aria-busy')==='true'`)
	run(chromedp.Evaluate(`window.dbLoadResponse()`, nil))
	wait(`document.querySelector('#db-data-loading').classList.contains('hidden') && document.querySelectorAll('#db-data-grid .tabulator-row').length===2`)
	click(`#db-reload-table`)
	wait(`!document.querySelector('#db-data-loading').classList.contains('hidden')`)
	click(`#db-data-load-stop`)
	wait(`document.querySelector('#db-data-loading').classList.contains('hidden') && !document.querySelector('#db-data-grid').inert`)
	check(`document.querySelectorAll('#db-data-grid .tabulator-row').length===2`)
	// Fail a SQL request; the loading state always clears.
	run(chromedp.Evaluate(`window.fetch=(url,options)=>String(url).includes('/api/database/query?')?Promise.reject(new Error('load failed fixture')):window.dbLoadFetch(url,options)`, nil))
	click(`#db-run-query`)
	wait(`document.querySelector('#db-query-loading').classList.contains('hidden') && document.querySelector('#db-query-summary').textContent.includes('load failed fixture')`)
}
