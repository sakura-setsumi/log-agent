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

func TestE2EDatabaseSevenTemplates(t *testing.T) {
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
			_ = chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#db-sql-editor').innerText+' | '+document.querySelector('#db-schema-modal').innerText`, &dump))
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
	check(`JSON.stringify([...document.querySelectorAll('[data-db-template]')].map(el=>el.dataset.dbTemplate))===JSON.stringify(['select','insert','update','delete','create','drop','alter'])`)
	click(`#db-close-templates`)
	run(chromedp.Evaluate(`window.__templateExecutions=0;const originalFetch=fetch;window.fetch=(url,options,...args)=>{const body=options?.body?JSON.parse(options.body):null;if(String(url).includes('/api/database/query') || String(url).includes('/api/database/schema') && body?.hash)window.__templateExecutions++;return originalFetch(url,options,...args)}`, nil))
	open := func(id string) {
		t.Helper()
		click(`#db-toggle-templates`)
		click(`[data-db-template="` + id + `"]`)
		wait(`!document.querySelector('#db-schema-modal').classList.contains('hidden') && !document.querySelector('#db-template-schema-schema').disabled && !!document.querySelector('#db-schema-fields').firstElementChild`)
		check(`document.querySelector('#db-apply-schema').textContent==='Fill SQL' && document.querySelector('#db-apply-schema').disabled`)
	}
	preview := func() {
		t.Helper()
		click(`#db-schema-form [type="submit"]`)
		wait(`!document.querySelector('#db-apply-schema').disabled && !document.querySelector('#db-schema-preview').classList.contains('hidden')`)
	}
	sourceTable := func(name string) {
		t.Helper()
		value, _ := json.Marshal(name)
		run(chromedp.Evaluate(`document.querySelector('#db-template-schema-table').value=`+string(value)+`;document.querySelector('#db-template-schema-table').dispatchEvent(new Event('change',{bubbles:true}))`, nil))
		wait(`!!document.querySelector('#db-schema-fields').firstElementChild && document.querySelector('#db-template-schema-table').value===` + string(value))
	}
	setSQL("SELECT 7 AS untouched;\nSELECT * FROM orders;")
	open("create")
	check(`document.querySelector('#db-template-schema-table-label').hidden`)
	run(chromedp.Evaluate(`document.querySelector('#db-schema-form [name="objectName"]').value='ui_template_new';document.querySelectorAll('[data-column-name]')[1].value='title';document.querySelectorAll('[data-column-type]')[1].value='text'`, nil))
	click(`#db-schema-fields [data-add-column]`)
	run(chromedp.Evaluate(`document.querySelectorAll('[data-column-name]')[2].value='amount';document.querySelectorAll('[data-column-type]')[2].value='numeric'`, nil))
	preview()
	if path := os.Getenv("LOG_AGENT_SEVEN_TEMPLATE_PREVIEW"); path != "" {
		var screenshot []byte
		run(chromedp.CaptureScreenshot(&screenshot))
		if err := os.WriteFile(path, screenshot, 0600); err != nil {
			t.Fatal(err)
		}
	}
	click(`#db-apply-schema`)
	hasSQL(`SELECT 7 AS untouched;`)
	hasSQL(`CREATE TABLE`)
	hasSQL(`ui_template_new`)
	hasSQL(`title`)
	hasSQL(`amount`)
	pool, _ := databases.pool(context.Background(), a)
	var count int
	if err := pool.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='ui_template_new'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("CREATE template executed: %d %v", count, err)
	}
	setSQL(`SELECT * FROM orders;`)
	open("drop")
	sourceTable("orders")
	preview()
	click(`#db-apply-schema`)
	hasSQL(`DROP TABLE`)
	hasSQL(`orders`)
	if err := pool.db.QueryRow(`SELECT count(*) FROM orders`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("DROP template executed: %d %v", count, err)
	}
	for _, action := range []string{"addColumn", "alterColumn", "dropColumn"} {
		t.Logf("ALTER action: %s",action)
        setSQL(`SELECT * FROM orders;`)
		open("alter")
		sourceTable("orders")
		check(`JSON.stringify([...document.querySelector('#db-schema-form [name="action"]').options].filter(o=>!o.hidden).map(o=>o.value))===JSON.stringify(['addColumn','alterColumn','dropColumn'])`)
		actionJSON, _ := json.Marshal(action)
		run(chromedp.Evaluate(`document.querySelector('#db-schema-form [name="action"]').value=`+string(actionJSON)+`;document.querySelector('#db-schema-form [name="action"]').dispatchEvent(new Event('change',{bubbles:true}))`, nil))
		switch action {
		case "addColumn":
			run(chromedp.Evaluate(`document.querySelector('[data-column-name]').value='extra';document.querySelector('[data-column-type]').value='text'`, nil))
		case "alterColumn":
			run(chromedp.Evaluate(`document.querySelector('#db-schema-form [name="oldName"]').value='note';document.querySelector('#db-schema-form [name="oldName"]').dispatchEvent(new Event('change',{bubbles:true}));document.querySelector('[data-column-type]').value='integer'`, nil))
		case "dropColumn":
			run(chromedp.Evaluate(`document.querySelector('#db-schema-form [name="oldName"]').value='note';document.querySelector('#db-schema-form [name="oldName"]').dispatchEvent(new Event('change',{bubbles:true}))`, nil))
		}
		preview()
		// Input changes invalidate the generated plan before SQL can be filled.
		if action == "addColumn" {
			run(chromedp.Evaluate(`document.querySelector('[data-column-name]').value='extra2';document.querySelector('[data-column-name]').dispatchEvent(new Event('input',{bubbles:true}))`, nil))
			check(`document.querySelector('#db-apply-schema').disabled`)
			preview()
		}
		click(`#db-apply-schema`)
		if action == "addColumn" {
			hasSQL(`ALTER TABLE`)
			hasSQL(`extra2`)
		}
		if action == "alterColumn" {
			hasSQL(`note`)
		}
		if action == "dropColumn" {
			hasSQL(`orders`)
		}
		// The original table still has exactly the original three columns.
		var actual int
		if err := pool.db.QueryRow(`SELECT count(*) FROM pragma_table_info('orders')`).Scan(&actual); err != nil || actual != 3 {
			t.Fatalf("ALTER template executed: %d %v", actual, err)
		}
	}
	check(`window.__templateExecutions===0`)
	// Reopening the normal structure designer restores its original apply action.
	click(`[data-db-connect="east"]`)
	wait(`!!document.querySelector('[data-db-database]')`)
	click(`[data-db-database]`)
	wait(`!!document.querySelector('[data-db-table]')`)
	click(`[data-db-table]`)
	wait(`!!document.querySelector('#db-create-table')`)
	click(`#db-create-table`)
	wait(`!document.querySelector('#db-schema-modal').classList.contains('hidden')`)
	check(`document.querySelector('#db-apply-schema').textContent==='Apply schema changes' && document.querySelector('#db-template-schema-targets').classList.contains('hidden')`)
}
