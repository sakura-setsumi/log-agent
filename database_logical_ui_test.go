package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

func TestE2EDatabaseLogicalConditions(t *testing.T) {
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
	run(chromedp.Navigate(web.URL), chromedp.WaitVisible(`.nav-item[data-view="database"]`, chromedp.ByQuery), chromedp.Click(`.nav-item[data-view="database"]`, chromedp.ByQuery))
	wait(`!!document.querySelector('[data-db-connect="east"]')`)
	run(chromedp.Click(`#db-target-select`, chromedp.ByQuery), chromedp.Click(`[data-db-target-group="east"]`, chromedp.ByQuery))
	wait(`document.querySelector('#db-target-select').dataset.count==='1'`)
	run(chromedp.Click(`#db-target-picker-close`, chromedp.ByQuery), chromedp.Focus(`#db-sql-editor .cm-content`, chromedp.ByQuery))
	typeText(`SELECT * FROM "orders" WHERE "note" = 'first'  LIMIT 100;`)
	insertLogical := func(logical string) {
		t.Helper()
		run(chromedp.Focus(`#db-sql-editor .cm-content`, chromedp.ByQuery), chromedp.KeyEvent(kb.End))
		for range " LIMIT 100;" {
			run(chromedp.KeyEvent(kb.ArrowLeft))
		}
		typeText(logical)
		wait(`!document.querySelector('#db-inline-picker').classList.contains('hidden')`)
	}
	insertLogical("AND")
	check(`!document.querySelector('#db-inline-table')`)
	wait(`!!document.querySelector('#db-inline-body [value="amount"]')`)
	run(chromedp.Click(`#db-inline-body [value="amount"]`, chromedp.ByQuery), chromedp.Click(`#db-inline-apply`, chromedp.ByQuery))
	wait(`!!document.querySelector('#db-inline-operator')`)
	run(chromedp.Evaluate(`document.querySelector('#db-inline-operator').value='>=';document.querySelector('#db-inline-operator').dispatchEvent(new Event('change'));document.querySelector('#db-inline-value').value='12.5'`, nil), chromedp.Click(`#db-inline-apply`, chromedp.ByQuery))
	hasSQL(`WHERE "note" = 'first' AND "amount" >= 12.5 LIMIT 100;`)
	insertLogical(" OR")
	wait(`!!document.querySelector('#db-inline-body [value="id"]')`)
	check(`!document.querySelector('#db-inline-table')`)
	run(chromedp.Click(`#db-inline-body [value="id"]`, chromedp.ByQuery), chromedp.Click(`#db-inline-apply`, chromedp.ByQuery))
	wait(`!!document.querySelector('#db-inline-operator')`)
	run(chromedp.Evaluate(`document.querySelector('#db-inline-operator').value='=';document.querySelector('#db-inline-operator').dispatchEvent(new Event('change'));document.querySelector('#db-inline-value').value='7'`, nil), chromedp.Click(`#db-inline-apply`, chromedp.ByQuery))
	check(`document.querySelector('#db-sql-editor .cm-content').textContent.includes('AND "amount" >= 12.5 OR "id" = 7 LIMIT 100;')`)
	// Each new condition remains independently editable.
	run(chromedp.Evaluate(`Array.from(document.querySelectorAll('[data-db-slot-kind="field"]')).find(el=>el.textContent==='"amount"').click()`, nil))
	wait(`!!document.querySelector('#db-inline-body [value="note"]')`)
	run(chromedp.Click(`#db-inline-body [value="note"]`, chromedp.ByQuery), chromedp.Click(`#db-inline-apply`, chromedp.ByQuery))
	hasSQL(`WHERE "note" = 'first' AND "note" >= 12.5 OR "id" = 7 LIMIT 100;`)
	resetSQL := func(sql string) {
		t.Helper()
		run(chromedp.Focus(`#db-sql-editor .cm-content`, chromedp.ByQuery), chromedp.ActionFunc(func(ctx context.Context) error {
			if err := input.DispatchKeyEvent(input.KeyRawDown).WithKey("a").WithCode("KeyA").WithWindowsVirtualKeyCode(65).WithModifiers(input.ModifierCtrl).Do(ctx); err != nil {
				return err
			}
			return input.DispatchKeyEvent(input.KeyUp).WithKey("a").WithCode("KeyA").WithWindowsVirtualKeyCode(65).WithModifiers(input.ModifierCtrl).Do(ctx)
		}))
		typeText(sql)
	}
	// A prefix OR while typing ORDER must not steal keyboard focus.
	resetSQL(`SELECT * FROM orders WHERE id = 1 `)
	for _, letter := range []string{"O", "R", "D", "E", "R"} {
		typeText(letter)
	}
	typeText(" BY id;")
	run(chromedp.Sleep(350 * time.Millisecond))
	check(`document.querySelector('#db-inline-picker').classList.contains('hidden') && !document.querySelector('[data-db-slot-kind="condition"]')`)
	for _, logical := range []string{"and", "or"} {
		resetSQL(`SELECT * FROM orders WHERE id = 1 `)
		typeText(logical)
		typeText(" ")
		wait(`!document.querySelector('#db-inline-picker').classList.contains('hidden')`)
		run(chromedp.KeyEvent(kb.Escape))
		check(`document.querySelector('#db-inline-picker').classList.contains('hidden')`)
	}

}
