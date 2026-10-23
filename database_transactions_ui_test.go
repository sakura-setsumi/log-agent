package main

import (
	"context"
	"encoding/json"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestE2EResultTransactions(t *testing.T) {
	browser := firstExistingPath(`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`)
	if browser == "" {
		t.Skip("Chrome or Edge required")
	}
	a, b := databaseTestSetup(t)
	east, _ := databases.pool(context.Background(), a)
	west, _ := databases.pool(context.Background(), b)
	for _, pool := range []*databasePool{east, west} {
		if _, err := pool.db.Exec("CREATE TABLE extra(id INTEGER PRIMARY KEY,note TEXT);INSERT INTO extra VALUES(1,'old')"); err != nil {
			t.Fatal(err)
		}
	}
	web := httptest.NewServer(newHTTPHandler(newServer()))
	defer web.Close()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(browser), chromedp.NoSandbox, chromedp.WindowSize(1500, 1000))
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
		if err := chromedp.Run(ctx, chromedp.Poll(expression, nil, chromedp.WithPollingTimeout(10*time.Second))); err != nil {
			var dump string
			_ = chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#db-panel-result').innerText`, &dump))
			t.Fatalf("%s: %v; %s", expression, err, dump)
		}
	}
	check := func(expression string) {
		t.Helper()
		var ok bool
		run(chromedp.Evaluate(expression, &ok))
		if !ok {
			var dump string
			_ = chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#db-panel-result').innerHTML`, &dump))
			t.Fatalf("%s: %s", expression, dump)
		}
	}
	setSQL := func(sql string) {
		t.Helper()
		run(chromedp.Focus(`#db-sql-editor .cm-content`, chromedp.ByQuery), chromedp.ActionFunc(func(ctx context.Context) error {
			if err := input.DispatchKeyEvent(input.KeyRawDown).WithKey("a").WithCode("KeyA").WithWindowsVirtualKeyCode(65).WithModifiers(input.ModifierCtrl).Do(ctx); err != nil {
				return err
			}
			if err := input.DispatchKeyEvent(input.KeyUp).WithKey("a").WithCode("KeyA").WithWindowsVirtualKeyCode(65).WithModifiers(input.ModifierCtrl).Do(ctx); err != nil {
				return err
			}
			return input.InsertText(sql).Do(ctx)
		}))
	}
	edit := func(selector, value string) {
		t.Helper()
		wait(`DatabaseUI.Tabulator.findTable('#db-query-grid')[0]?._databaseReady === true`)
		quoted, _ := json.Marshal(selector)
		run(chromedp.Evaluate("document.querySelector("+string(quoted)+").click()", nil), chromedp.WaitVisible(`#db-query-grid .tabulator-editing input`, chromedp.ByQuery), chromedp.ActionFunc(func(ctx context.Context) error { return input.InsertText(value).Do(ctx) }))
		run(chromedp.KeyEvent(kb.Enter))
		wait(`!document.querySelector('#db-query-commit').disabled`)
	}
	note := func(pool *databasePool, table string) any {
		t.Helper()
		var value any
		if err := pool.db.QueryRow("SELECT note FROM " + table + " WHERE id=1").Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	run(chromedp.Navigate(web.URL), chromedp.WaitVisible(`.nav-item[data-view="database"]`, chromedp.ByQuery), chromedp.Click(`.nav-item[data-view="database"]`, chromedp.ByQuery))
	wait(`!!document.querySelector('[data-db-connect="east"]')`)
	run(chromedp.Click(`#db-target-select`, chromedp.ByQuery), chromedp.Click(`[data-db-target-group="east"]`, chromedp.ByQuery), chromedp.Click(`[data-db-target-group="west"]`, chromedp.ByQuery))
	wait(`document.querySelector('#db-target-select').dataset.count==='2'`)
	run(chromedp.Click(`#db-target-picker-close`, chromedp.ByQuery))
	setSQL("SELECT id,note FROM orders ORDER BY id;")
	run(chromedp.Click(`#db-run-query`, chromedp.ByQuery))
	wait(`document.querySelectorAll('#db-query-grid .tabulator-row').length===4`)
	check(`!document.querySelector('#db-query-grid input[type="checkbox"]') && document.querySelector('#db-query-grid .tabulator-col[tabulator-field="__select"]').getBoundingClientRect().width===9`)
	// Row zero is a valid anchor: Shift and Ctrl use the same row header logic as table browsing.
	run(chromedp.Evaluate(`document.querySelector('#db-query-grid .tabulator-row:nth-child(1) [tabulator-field="__select"]').click();document.querySelector('#db-query-grid .tabulator-row:nth-child(3) [tabulator-field="__select"]').dispatchEvent(new MouseEvent('click',{bubbles:true,shiftKey:true}))`, nil))
	check(`document.querySelectorAll('#db-query-grid .tabulator-row.tabulator-selected').length===3`)
	run(chromedp.Evaluate(`document.querySelector('#db-query-grid .tabulator-row:nth-child(2) [tabulator-field="__select"]').dispatchEvent(new MouseEvent('click',{bubbles:true,ctrlKey:true}))`, nil))
	check(`document.querySelectorAll('#db-query-grid .tabulator-row.tabulator-selected').length===2`)
	run(chromedp.Evaluate(`document.querySelector('#db-query-grid .tabulator-row:nth-child(4) [tabulator-field="__select"]').click()`, nil))
	check(`document.querySelectorAll('#db-query-grid .tabulator-row.tabulator-selected').length===1 && document.querySelectorAll('#db-query-grid .tabulator-row')[3].classList.contains('tabulator-selected')`)

	edit(`#db-query-grid .tabulator-row:first-child [tabulator-field="v1"]`, "first edit")
	if note(east, "orders") != nil {
		t.Fatal("result edit auto-committed")
	}
	run(chromedp.Click(`#db-query-grid .tabulator-row:nth-child(2) [tabulator-field="__select"]`, chromedp.ByQuery), chromedp.Click(`#db-query-delete`, chromedp.ByQuery))
	wait(`document.querySelectorAll('#db-query-grid .db-row-deleted').length===1`)
	if path := os.Getenv("LOG_AGENT_TRANSACTIONS_PREVIEW"); path != "" {
		var screenshot []byte
		run(chromedp.CaptureScreenshot(&screenshot))
		if err := os.WriteFile(path, screenshot, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	_ = east.db.QueryRow("SELECT COUNT(*) FROM orders").Scan(&count)
	if count != 2 {
		t.Fatal("deletion auto-committed")
	}
	run(chromedp.Click(`#db-query-rollback`, chromedp.ByQuery))
	wait(`document.querySelector('#db-query-commit').disabled && !document.querySelector('#db-query-grid .db-row-deleted')`)
	wait(`!!document.querySelector('#db-query-grid .tabulator-row:first-child [tabulator-field="v1"] .db-cell-null')`)
	edit(`#db-query-grid .tabulator-row:first-child [tabulator-field="v1"]`, "first edit")
	run(chromedp.Click(`#db-query-grid .tabulator-row:nth-child(2) [tabulator-field="__select"]`, chromedp.ByQuery), chromedp.Click(`#db-query-delete`, chromedp.ByQuery))
	run(chromedp.Click(`#db-add-sql-tab`, chromedp.ByQuery))
	setSQL("SELECT id,note FROM extra;")
	run(chromedp.Click(`#db-run-query`, chromedp.ByQuery))
	wait(`document.querySelectorAll('#db-query-grid .tabulator-row').length===2`)
	edit(`#db-query-grid .tabulator-row:first-child [tabulator-field="v1"]`, "east extra")
	edit(`#db-query-grid .tabulator-row:nth-child(2) [tabulator-field="v1"]`, "west extra")
	if note(east, "extra") != "old" || note(west, "extra") != "old" {
		t.Fatal("second result edits auto-committed")
	}
	run(chromedp.Click(`#db-query-commit-all`, chromedp.ByQuery))
	wait(`document.querySelector('#db-query-commit-all').disabled && !document.querySelector('#db-query-grid').inert`)
	if note(east, "orders") != "first edit" || note(east, "extra") != "east extra" || note(west, "extra") != "west extra" {
		t.Fatalf("multi-page commit values: orders=%v east=%v west=%v", note(east, "orders"), note(east, "extra"), note(west, "extra"))
	}
	_ = east.db.QueryRow("SELECT COUNT(*) FROM orders").Scan(&count)
	if count != 1 {
		t.Fatal("result deletion was not committed")
	}
	// A failure in one database rolls back all its result pages, while another
	// database commits successfully and is not resent on retry.
	edit(`#db-query-grid .tabulator-row:first-child [tabulator-field="v1"]`, "east pending")
	edit(`#db-query-grid .tabulator-row:nth-child(2) [tabulator-field="v1"]`, "west committed")
	if _, err := east.db.Exec("UPDATE extra SET note='external' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	run(chromedp.Click(`[data-db-bottom="sql-1"]`, chromedp.ByQuery))
	wait(`document.querySelectorAll('#db-query-grid .tabulator-row').length===3`)
	edit(`#db-query-grid .tabulator-row:first-child [tabulator-field="v1"]`, "must roll back")
	run(chromedp.Click(`#db-query-commit-all`, chromedp.ByQuery))
	wait(`document.querySelector('#db-query-summary').textContent.includes('record changed') && !document.querySelector('#db-query-grid').inert`)
	if note(east, "orders") != "first edit" || note(east, "extra") != "external" || note(west, "extra") != "west committed" {
		t.Fatal("partial transaction outcome was incorrect")
	}
	check(`!document.querySelector('#db-query-commit').disabled`)
	run(chromedp.Click(`#db-query-rollback`, chromedp.ByQuery), chromedp.Click(`[data-db-bottom="sql-2"]`, chromedp.ByQuery))
	wait(`document.querySelector('#db-query-pending').textContent.includes('1 pending')`)
	wait(`document.querySelector('#db-query-grid .tabulator-row:nth-child(2) [tabulator-field="v1"]')?.textContent==='west committed'`)
	run(chromedp.Click(`#db-query-rollback`, chromedp.ByQuery))
	wait(`document.querySelector('#db-query-commit-all').disabled`)
}
