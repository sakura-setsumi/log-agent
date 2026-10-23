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

func TestE2EDatabaseIncrementalSelectors(t *testing.T) {
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
	hasSQL := func(sql string) {
		t.Helper()
		quoted, _ := json.Marshal(sql)
		check("document.querySelector('#db-sql-editor .cm-content').textContent.includes(" + string(quoted) + ")")
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
	click(`#db-toggle-templates`)
	check(`document.querySelectorAll('[data-db-template]').length===7 && !document.querySelector('[data-db-template="where"], [data-db-template="count"], [data-db-template="order"], [data-db-template="distinct"], [data-db-template="group"]')`)
	click(`#db-close-templates`)
	run(chromedp.Evaluate(`window.__incrementQueries=0;const originalFetch=fetch;window.fetch=(url,...args)=>{if(String(url).includes('/api/database/query'))window.__incrementQueries++;return originalFetch(url,...args)}`, nil))
	// Typing a keyword at the end still inserts before LIMIT in the current statement.
	setSQL(`SELECT * FROM orders WHERE id=1 LIMIT 100 `)
	typeText("count")
	wait(`!!document.querySelector('#db-count-column')`)
	check(`!document.querySelector('#db-inline-table') && !!document.querySelector('#db-result-alias')`)
	run(chromedp.Evaluate(`document.querySelector('#db-count-column').value='id';document.querySelector('#db-result-alias').value='匹配行数'`, nil))
	if path := os.Getenv("LOG_AGENT_INCREMENT_PREVIEW"); path != "" {
		var screenshot []byte
		run(chromedp.CaptureScreenshot(&screenshot))
		if err := os.WriteFile(path, screenshot, 0600); err != nil {
			t.Fatal(err)
		}
	}
	click(`#db-inline-apply`)
	hasSQL(`SELECT COUNT("id") AS "匹配行数" FROM orders WHERE id=1 LIMIT 100`)
	click(`[data-db-slot-kind="alias"]`)
	wait(`!!document.querySelector('#db-result-alias')`)
	run(chromedp.Evaluate(`document.querySelector('#db-result-alias').value='total'`, nil))
	click(`#db-inline-apply`)
	hasSQL(`COUNT("id") AS "total"`)
	click(`[data-db-slot-kind="countField"]`)
	wait(`!!document.querySelector('#db-count-column')`)
	check(`document.querySelector('#db-count-column').value==='id' && document.querySelector('#db-result-alias').value==='total'`)
	run(chromedp.Evaluate(`document.querySelector('#db-count-column').value='*'`, nil))
	click(`#db-inline-apply`)
	hasSQL(`COUNT(*) AS "total"`)
	for _, keyword := range []string{"where", "ORDER", "group", "DISTINCT"} {
		setSQL(`SELECT * FROM orders WHERE id=1 LIMIT 100 `)
		// Type a word character by character, as in the native editor.
		for _, letter := range keyword {
			typeText(string(letter))
		}
		wait(`!!document.querySelector('#db-inline-body [name="inline-column"]') && !document.querySelector('#db-inline-picker').classList.contains('hidden')`)
		check(`!document.querySelector('#db-inline-table')`)
		run(chromedp.Evaluate(`document.querySelectorAll('[name="inline-column"]').forEach(input=>input.checked=input.value==='amount')`, nil))
		if keyword == "ORDER" {
			run(chromedp.Evaluate(`document.querySelector('#db-sort-direction').value='DESC'`, nil))
		}
		click(`#db-inline-apply`)
		switch keyword {
		case "where":
			wait(`!!document.querySelector('#db-inline-operator')`)
			run(chromedp.Evaluate(`document.querySelector('#db-inline-operator').value='>=';document.querySelector('#db-inline-operator').dispatchEvent(new Event('change'));document.querySelector('#db-inline-value').value='12.5'`, nil))
			click(`#db-inline-apply`)
			hasSQL(`WHERE id=1 AND "amount" >= 12.5 LIMIT 100`)
		case "ORDER":
			hasSQL(`WHERE id=1 ORDER BY "amount" DESC LIMIT 100`)
		case "group":
			hasSQL(`WHERE id=1 GROUP BY "amount" LIMIT 100`)
		case "DISTINCT":
			hasSQL(`SELECT DISTINCT "amount" FROM orders WHERE id=1 LIMIT 100`)
		}
	}
	setSQL("SELECT * FROM orders WHERE id=220;\nSELECT * FROM orders WHERE id=197 ")
	typeText("count")
	wait(`!!document.querySelector('#db-count-column')`)
	click(`#db-inline-apply`)
	hasSQL(`SELECT * FROM orders WHERE id=220;`)
	hasSQL(`SELECT COUNT(*) AS "row_count" FROM orders WHERE id=197`)
	// Literal text never opens a selector.
	setSQL(`SELECT * FROM orders WHERE note='`)
	typeText("count")
	run(chromedp.Sleep(350 * time.Millisecond))
	check(`document.querySelector('#db-inline-picker').classList.contains('hidden') && window.__incrementQueries===0`)
	// A legacy saved COUNT had only a table slot. Restore its new selectors too.
	run(chromedp.Evaluate(`(() => {
      const sql='SELECT COUNT(*) AS row_count FROM orders;', from=sql.indexOf('orders');
      const saved={tabs:[{id:'sql-1',name:'SQL 1',text:sql,slotState:{slots:[{id:'old-table',kind:'table',from,to:from+6,template:{id:'template-1',templateId:'count',context:null}}]}}],active:'sql-1',sequence:1};
      Storage.prototype.setItem.call(localStorage,'log-agent-database-sql-tabs',JSON.stringify(saved));Storage.prototype.setItem=()=>{};
    })()`, nil), chromedp.Reload(), chromedp.WaitVisible(`.nav-item[data-view="database"]`, chromedp.ByQuery))
	click(`.nav-item[data-view="database"]`)
	wait(`!!document.querySelector('[data-db-slot-kind="countField"]') && !!document.querySelector('[data-db-slot-kind="alias"]')`)
	click(`[data-db-slot-kind="alias"]`)
	wait(`!!document.querySelector('#db-result-alias')`)
	check(`document.querySelector('#db-result-alias').value==='row_count'`)
	run(chromedp.Evaluate(`document.querySelector('#db-result-alias').value='恢复统计'`, nil))
	click(`#db-inline-apply`)
	hasSQL(`AS "恢复统计" FROM orders;`)
}
