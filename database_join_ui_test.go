package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

func TestE2EDatabaseJoinSelectors(t *testing.T) {
	browser := firstExistingPath(`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`)
	if browser == "" {
		t.Skip("Chrome or Edge required")
	}
	a, _ := databaseTestSetup(t)
	pool, _ := databases.pool(context.Background(), a)
	if _, err := pool.db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, team_id INTEGER, tenant_id INTEGER); CREATE TABLE teams (id INTEGER PRIMARY KEY, tenant_id INTEGER); INSERT INTO users VALUES (1,10,7); INSERT INTO teams VALUES (10,7);`); err != nil {
		t.Fatal(err)
	}
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
			var dump string
			_ = chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#db-sql-editor').innerText+' | '+document.querySelector('#db-inline-picker').innerText`, &dump))
			t.Log(dump)
			t.Fatal(expression)
		}
	}
	hasSQL := func(sql string) {
		t.Helper()
		quoted, _ := json.Marshal(sql)
		check("document.querySelector('#db-sql-editor .cm-content').textContent.replace(/\\s+/g,' ').includes(" + string(quoted) + ")")
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
	run(chromedp.Evaluate(`window.__joinQueries=0;const originalFetch=fetch;window.fetch=(url,...args)=>{if(String(url).includes('/api/database/query'))window.__joinQueries++;return originalFetch(url,...args)}`, nil))
	chooseTable := func(name string) {
		t.Helper()
		value, _ := json.Marshal("." + name)
		run(chromedp.Evaluate(`(() => {const select=document.querySelector('#db-join-table');select.value=[...select.options].find(o=>o.textContent.endsWith(`+string(value)+`)).value;select.dispatchEvent(new Event('change',{bubbles:true}));})()`, nil))
		wait(`!!document.querySelector('[data-join-right-field]') && !document.querySelector('[data-join-right-field]').disabled && !document.querySelector('#db-inline-apply').disabled`)
	}
	ready := func() {
		t.Helper()
		wait(`!document.querySelector('#db-inline-picker').classList.contains('hidden') && !!document.querySelector('#db-join-alias') && !!document.querySelector('[data-join-left-field]') && !document.querySelector('[data-join-left-field]').disabled && !document.querySelector('#db-inline-apply').disabled`)
	}
	setSQL("SELECT 42 AS untouched;\nSELECT o.* FROM orders o WHERE o.id = 1 ORDER BY o.id LIMIT 10 ")
	typeText("inner")
	ready()
	chooseTable("users")
	check(`document.querySelector('#db-join-left-table').options.length===1 && document.querySelector('#db-join-alias').value==='t2'`)
	run(chromedp.Evaluate(`document.querySelector('#db-join-alias').value='o';document.querySelector('#db-join-alias').dispatchEvent(new Event('input',{bubbles:true}))`, nil))
	check(`document.querySelector('#db-inline-apply').disabled`)
	run(chromedp.Evaluate(`document.querySelector('#db-join-alias').value='t2';document.querySelector('#db-join-alias').dispatchEvent(new Event('input',{bubbles:true}));document.querySelector('[data-join-left-field]').value='id';document.querySelector('[data-join-right-field]').value='id'`, nil))
	if path := os.Getenv("LOG_AGENT_JOIN_PREVIEW"); path != "" {
		var screenshot []byte
		run(chromedp.CaptureScreenshot(&screenshot))
		if err := os.WriteFile(path, screenshot, 0600); err != nil {
			t.Fatal(err)
		}
	}
	click(`#db-inline-apply`)
	hasSQL(`INNER JOIN "users" AS "t2" ON o."id" = "t2"."id" WHERE o.id = 1 ORDER BY o.id LIMIT 10`)
	// No caret movement: the previous application leaves room to type the next JOIN.
	typeText("left")
	ready()
	chooseTable("teams")
	check(`document.querySelector('#db-join-left-table').options.length===2 && document.querySelector('#db-join-left-table').value==='1' && document.querySelector('#db-join-alias').value==='t3'`)
	run(chromedp.Evaluate(`document.querySelector('[data-join-left-field]').value='team_id';document.querySelector('[data-join-right-field]').value='id'`, nil))
	click(`#db-join-add-condition`)
	wait(`document.querySelectorAll('[data-join-condition]').length===2`)
	run(chromedp.Evaluate(`document.querySelectorAll('[data-join-left-field]')[1].value='tenant_id';document.querySelectorAll('[data-join-right-field]')[1].value='tenant_id'`, nil))
	click(`#db-inline-apply`)
	hasSQL(`LEFT JOIN "teams" AS "t3" ON "t2"."team_id" = "t3"."id" AND "t2"."tenant_id" = "t3"."tenant_id" WHERE o.id = 1`)
	// Reusing a table gives it a new alias, and any earlier table can be the source.
	typeText("RIGHT OUTER JOIN")
	ready()
	chooseTable("users")
	check(`document.querySelector('#db-join-left-table').options.length===3 && document.querySelector('#db-join-alias').value==='t4'`)
	run(chromedp.Evaluate(`document.querySelector('#db-join-left-table').value='0';document.querySelector('#db-join-left-table').dispatchEvent(new Event('change',{bubbles:true}))`, nil))
	wait(`!document.querySelector('[data-join-left-field]').disabled && !document.querySelector('#db-inline-apply').disabled`)
	run(chromedp.Evaluate(`document.querySelector('[data-join-left-field]').value='id';document.querySelector('[data-join-right-field]').value='id'`, nil))
	click(`#db-inline-apply`)
	hasSQL(`RIGHT JOIN "users" AS "t4" ON o."id" = "t4"."id" WHERE o.id = 1 ORDER BY o.id LIMIT 10`)
	hasSQL(`SELECT 42 AS untouched;`)
	check(`window.__joinQueries===0 && !document.querySelector('[data-db-slot-kind="incremental"]')`)
	var sql string
	run(chromedp.Evaluate(`[...document.querySelectorAll('#db-sql-editor .cm-line')].at(-1).textContent`, &sql))
	// Execute only in the isolated test database to verify the composed SQL is valid.
	rows, err := pool.db.QueryContext(context.Background(), strings.TrimSpace(sql))
	if err != nil {
		t.Fatalf("generated JOIN SQL invalid: %s: %v", sql, err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("generated JOIN did not return the expected row: %s", sql)
	}
	var id int64
	var amount float64
	var note any
	if err := rows.Scan(&id, &amount, &note); err != nil || id != 1 {
		t.Fatalf("joined row: %d %v", id, err)
	}
}
