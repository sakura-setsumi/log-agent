package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

func TestE2EDatabaseWorkspace(t *testing.T) {
	browser := firstExistingPath(`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`)
	if browser == "" {
		t.Skip("Chrome or Edge required")
	}
	a, _ := databaseTestSetup(t)
	web := httptest.NewServer(newHTTPHandler(newServer()))
	t.Cleanup(web.Close)
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(browser), chromedp.NoSandbox, chromedp.WindowSize(1600, 1000))
	allocator, closeAllocator := chromedp.NewExecAllocator(context.Background(), opts...)
	t.Cleanup(closeAllocator)
	ctx, closeBrowser := chromedp.NewContext(allocator, chromedp.WithErrorf(func(string, ...any) {}))
	t.Cleanup(closeBrowser)
	ctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()
	var exceptionMu sync.Mutex
	var exceptions []string
	chromedp.ListenTarget(ctx, func(event any) {
		if value, ok := event.(*runtime.EventExceptionThrown); ok {
			exceptionMu.Lock()
			detail := value.ExceptionDetails.Text
			if value.ExceptionDetails.Exception != nil {
				detail += ": " + value.ExceptionDetails.Exception.Description
			}
			exceptions = append(exceptions, detail)
			exceptionMu.Unlock()
		}
	})
	run := func(actions ...chromedp.Action) {
		t.Helper()
		if err := chromedp.Run(ctx, actions...); err != nil {
			t.Fatal(err)
		}
	}
	wait := func(expression, description string) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			var ok bool
			if err := chromedp.Run(ctx, chromedp.Evaluate(expression, &ok)); err == nil && ok {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		var dump string
		_ = chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#database-view').innerText + '\nDIALOG: ' + document.querySelector('#db-schema-modal').innerText`, &dump))
		t.Fatalf("waiting for %s: %s", description, dump)
	}
	run(chromedp.Navigate(web.URL), chromedp.WaitVisible(`.nav-item[data-view="database"]`, chromedp.ByQuery), chromedp.Click(`.nav-item[data-view="database"]`, chromedp.ByQuery))
	wait(`!!document.querySelector('[data-db-connect="east"]')`, "connections")
	toggleDatabase := func(id string) {
		t.Helper()
		value, _ := json.Marshal(id)
		run(chromedp.Evaluate("(() => { if(document.querySelector('#db-target-select').getAttribute('aria-expanded')!=='true')document.querySelector('#db-target-select').click();const branch=Array.from(document.querySelectorAll('[data-db-target-expand]')).find(button=>button.dataset.dbTargetExpand==="+string(value)+");if(branch.getAttribute('aria-expanded')!=='true')branch.click(); })()", nil))
		selector := `[data-db-check='["` + id + `","main"]']`
		run(chromedp.WaitVisible(selector, chromedp.ByQuery), chromedp.Click(selector, chromedp.ByQuery), chromedp.Click(`#db-target-picker-close`, chromedp.ByQuery))
	}
	// Targets live in an independent tree dropdown, without expanding or
	// changing the left object browser. Connection checkboxes load and select
	// their databases as a group.
	run(chromedp.Click(`#db-target-select`, chromedp.ByQuery), chromedp.Click(`[data-db-target-group="east"]`, chromedp.ByQuery))
	wait(`document.querySelector('#db-target-select').dataset.count==='1' && document.querySelector('[data-db-target-group="east"]').checked`, "lazy connection selection")
	run(chromedp.Click(`[data-db-target-group="west"]`, chromedp.ByQuery))
	wait(`document.querySelector('#db-target-select').dataset.count==='2' && document.querySelector('[data-db-target-group="west"]').checked`, "tree multi-selection")
	var cleanBrowser bool
	run(chromedp.Evaluate(`!document.querySelector('#db-tree input[type="checkbox"]') && !document.querySelector('#db-tree [data-db-database]')`, &cleanBrowser))
	if !cleanBrowser {
		t.Fatal("target selection added checkboxes to or expanded the left browser")
	}
	run(chromedp.Evaluate(`document.querySelector('#db-target-search').value='华东';document.querySelector('#db-target-search').dispatchEvent(new Event('input',{bubbles:true}))`, nil))
	wait(`document.querySelectorAll('#db-target-tree [data-db-check]').length===1 && document.querySelector('#db-target-select').dataset.count==='2'`, "search preserves hidden selections")
	run(chromedp.Evaluate(`document.querySelector('#db-target-search').value='';document.querySelector('#db-target-search').dispatchEvent(new Event('input',{bubbles:true}))`, nil), chromedp.KeyEvent(kb.Escape))
	wait(`document.querySelector('#db-target-picker').classList.contains('hidden') && document.activeElement.id==='db-target-select'`, "Escape closes selector and restores focus")
	run(chromedp.Click(`[data-db-connect="east"]`, chromedp.ByQuery), chromedp.Click(`[data-db-connect="west"]`, chromedp.ByQuery))
	wait(`document.querySelectorAll('#db-tree [data-db-database]').length===2`, "left database browser")
	run(chromedp.Click(`#db-target-select`, chromedp.ByQuery), chromedp.Click(`#db-target-picker-clear`, chromedp.ByQuery), chromedp.Click(`#db-target-picker-close`, chromedp.ByQuery), chromedp.Click(`[data-db-database='["east","main"]']`, chromedp.ByQuery))
	wait(`document.querySelector('#db-target-select').dataset.count==='0' && !document.querySelector('#db-tree input[type="checkbox"]')`, "clearing targets does not fall back to the browsed database")
	run(chromedp.Click(`[data-db-database='["east","main"]']`, chromedp.ByQuery))
	toggleDatabase("east")
	toggleDatabase("west")
	run(chromedp.Click(`#db-add-sql-tab`, chromedp.ByQuery))
	setSQL := func(text string) {
		t.Helper()
		run(chromedp.Focus(`#db-sql-editor .cm-content`, chromedp.ByQuery), chromedp.ActionFunc(func(ctx context.Context) error {
			if err := input.DispatchKeyEvent(input.KeyRawDown).WithKey("a").WithCode("KeyA").WithWindowsVirtualKeyCode(65).WithModifiers(input.ModifierCtrl).Do(ctx); err != nil {
				return err
			}
			if err := input.DispatchKeyEvent(input.KeyUp).WithKey("a").WithCode("KeyA").WithWindowsVirtualKeyCode(65).WithModifiers(input.ModifierCtrl).Do(ctx); err != nil {
				return err
			}
			return input.InsertText(text).Do(ctx)
		}))
	}
	setSQL("SELECT * FROM orders ORDER BY id")
	run(chromedp.Click(`#db-run-query`, chromedp.ByQuery))
	wait(`document.querySelectorAll('#db-query-grid .tabulator-row').length===4 && document.querySelector('#db-query-summary').textContent.includes('2/2')`, "four combined rows")
	var merged string
	run(chromedp.Text(`#db-query-grid`, &merged, chromedp.ByQuery))
	if !strings.Contains(merged, "华东") || !strings.Contains(merged, "华西") || !strings.Contains(merged, "9223372036854775807") || !strings.Contains(merged, "currency") {
		t.Fatalf("merged grid lost provenance, columns or integer precision: %s", merged)
	}
	// Editing a merged row writes only to that row's source, then retains NULL
	// handling and reports an optimistic-lock conflict without losing data.
	run(chromedp.DoubleClick(`#db-query-grid .tabulator-row:first-child [tabulator-field="v3"]`, chromedp.ByQuery))
	var missingColumnReadOnly bool
	run(chromedp.Evaluate(`!document.querySelector('#db-query-grid .tabulator-editing input')`, &missingColumnReadOnly))
	if !missingColumnReadOnly {
		t.Fatal("a column missing from this source is editable")
	}
	editResultNote := func(value string) {
		t.Helper()
		run(chromedp.DoubleClick(`#db-query-grid .tabulator-row:nth-child(3) [tabulator-field="v2"]`, chromedp.ByQuery), chromedp.WaitVisible(`#db-query-grid .tabulator-editing input`, chromedp.ByQuery), chromedp.ActionFunc(func(ctx context.Context) error { return input.InsertText(value).Do(ctx) }), chromedp.KeyEvent(kb.Enter))
	}
	editResultNote("result edit")
	wait(`document.querySelector('#db-query-summary .db-query-edit-status').textContent.includes('Saved:') && document.querySelector('#db-query-grid .tabulator-row:nth-child(3) [tabulator-field="v2"]').textContent==='result edit' && !document.querySelector('#db-query-grid').inert`, "query result cell saved")
	resultEastPool, _ := databases.pool(context.Background(), databaseTarget{ID: "east", Database: "main"})
	resultWestPool, _ := databases.pool(context.Background(), databaseTarget{ID: "west", Database: "main"})
	var resultEastNote, resultWestNote any
	if err := resultEastPool.db.QueryRow("SELECT note FROM orders WHERE id=1").Scan(&resultEastNote); err != nil || resultEastNote != nil {
		t.Fatalf("result edit touched east: %v %v", resultEastNote, err)
	}
	if err := resultWestPool.db.QueryRow("SELECT note FROM orders WHERE id=1").Scan(&resultWestNote); err != nil || resultWestNote != "result edit" {
		t.Fatalf("result edit did not reach west: %v %v", resultWestNote, err)
	}
	run(chromedp.Evaluate(`(() => { const cell=document.querySelector('#db-query-grid .tabulator-row:nth-child(3) [tabulator-field="v2"]'), r=cell.getBoundingClientRect();cell.dispatchEvent(new MouseEvent('contextmenu',{bubbles:true,cancelable:true,button:2,clientX:r.x+10,clientY:r.y+10})); })()`, nil), chromedp.WaitVisible(`.tabulator-menu-item`, chromedp.ByQuery), chromedp.Click(`.tabulator-menu-item`, chromedp.ByQuery))
	wait(`!document.querySelector('#db-query-grid').inert && !!document.querySelector('#db-query-grid .tabulator-row:nth-child(3) [tabulator-field="v2"] .db-cell-null')`, "query result set to NULL")
	if err := resultWestPool.db.QueryRow("SELECT note FROM orders WHERE id=1").Scan(&resultWestNote); err != nil || resultWestNote != nil {
		t.Fatalf("result NULL was not saved: %v %v", resultWestNote, err)
	}
	if _, err := resultWestPool.db.Exec("UPDATE orders SET note='external edit' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	editResultNote("stale edit")
	wait(`document.querySelector('#db-query-summary .db-query-edit-status.db-error')?.textContent.includes('record changed') && !document.querySelector('#db-query-grid').inert && !!document.querySelector('#db-query-grid .tabulator-row:nth-child(3) [tabulator-field="v2"] .db-cell-null')`, "failed result edit restores the displayed value")
	if err := resultWestPool.db.QueryRow("SELECT note FROM orders WHERE id=1").Scan(&resultWestNote); err != nil || resultWestNote != "external edit" {
		t.Fatalf("stale result overwrote live data: %v %v", resultWestNote, err)
	}
	if _, err := resultWestPool.db.Exec("UPDATE orders SET note=NULL WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	setSQL("-- semicolon ; in a comment\nSELECT 'left;right' AS marker;\n\nSELECT id FROM orders ORDER BY id;")
	wait(`document.querySelectorAll('.db-statement-action[data-scope="single"]').length===2 && document.querySelectorAll('.db-statement-action[data-scope="multi"]:enabled').length===2`, "per-statement single and multi query actions")
	run(chromedp.Click(`.db-statement-action[data-scope="single"]`, chromedp.ByQuery))
	wait(`document.querySelectorAll('#db-query-grid .tabulator-row').length===1 && document.querySelector('#db-query-summary').textContent.includes('1/1') && document.querySelector('#db-query-grid').textContent.includes('left;right')`, "single statement query preserved the quoted semicolon")
	run(chromedp.Evaluate(`document.querySelector('#db-single-target').value='["west","main"]';document.querySelector('#db-single-target').dispatchEvent(new Event('change', {bubbles:true}));Array.from(document.querySelectorAll('.db-statement-action[data-scope="single"]')).at(-1).click()`, nil))
	wait(`document.querySelectorAll('#db-query-grid .tabulator-row').length===2 && document.querySelector('#db-query-summary').textContent.includes('1/1') && !document.querySelector('#db-query-grid').textContent.includes('华东') && document.querySelector('#db-query-grid').textContent.includes('华西')`, "explicit single-database target")
	var checkedTargetsRemain bool
	run(chromedp.Evaluate(`document.querySelector('#db-target-select').dataset.count==='2' && document.querySelectorAll('[data-db-check]:checked').length===2`, &checkedTargetsRemain))
	if !checkedTargetsRemain {
		t.Fatal("single-database query changed multi-database selections")
	}
	run(chromedp.Evaluate(`Array.from(document.querySelectorAll('.db-statement-action[data-scope="multi"]')).at(-1).click()`, nil))
	wait(`document.querySelectorAll('#db-query-grid .tabulator-row').length===4 && document.querySelector('#db-query-summary').textContent.includes('2/2') && document.querySelector('#db-query-grid').textContent.includes('9223372036854775807')`, "only the chosen statement ran across both databases")
	toggleDatabase("west")
	wait(`document.querySelectorAll('.db-statement-action[data-scope="multi"]:enabled').length===0 && document.querySelectorAll('.db-statement-action[data-scope="single"]:enabled').length===2`, "multi query disabled with one database")
	toggleDatabase("west")
	setSQL("SELECT 1 AS first_statement;\nSELECT COUNT(*) AS current_statement FROM orders;")
	run(chromedp.KeyEvent(kb.Enter, chromedp.KeyModifiers(input.ModifierCtrl)))
	wait(`document.querySelectorAll('#db-query-grid .tabulator-row').length===2 && document.querySelector('#db-query-grid').textContent.includes('current_statement') && !document.querySelector('#db-query-grid').textContent.includes('first_statement')`, "Ctrl+Enter queried the statement at the cursor")
	// The same gutter controls also execute DML and DDL. Only the chosen
	// statement and chosen scope may change, and partial writes remain visible.
	setSQL("UPDATE orders SET amount=500;\nCREATE TABLE ui_cross_scope(id INTEGER PRIMARY KEY, amount INTEGER);")
	wait(`document.querySelectorAll('.db-statement-action[data-scope="multi"]:enabled').length===2`, "write and schema statement actions")
	run(chromedp.Evaluate(`Array.from(document.querySelectorAll('.db-statement-action[data-scope="multi"]')).at(-1).click()`, nil))
	wait(`document.querySelector('#db-query-summary').textContent.includes('2/2') && document.querySelectorAll('#db-query-grid .tabulator-row').length===2 && document.querySelector('#db-query-grid').textContent.includes('Affected rows')`, "multi-database DDL execution report")
	setSQL("INSERT INTO ui_cross_scope(id, amount) VALUES(1, 1);")
	run(chromedp.Click(`.db-statement-action[data-scope="multi"]`, chromedp.ByQuery))
	wait(`document.querySelector('#db-query-summary').textContent.includes('2 affected') && document.querySelector('#db-query-summary').textContent.includes('2/2')`, "multi-database insert affected counts")
	setSQL("UPDATE ui_cross_scope SET amount=amount+1 WHERE id=1;")
	run(chromedp.Evaluate(`document.querySelector('#db-single-target').value='["west","main"]';document.querySelector('#db-single-target').dispatchEvent(new Event('change', {bubbles:true}))`, nil))
	run(chromedp.Click(`.db-statement-action[data-scope="single"]`, chromedp.ByQuery))
	wait(`document.querySelector('#db-query-summary').textContent.includes('1/1') && document.querySelector('#db-query-grid').textContent.includes('华西')`, "single-database write uses selected target")
	for _, id := range []string{"east", "west"} {
		pool, _ := databases.pool(context.Background(), databaseTarget{ID: id, Database: "main"})
		var value int
		expected := 1
		if id == "west" {
			expected = 2
		}
		if err := pool.db.QueryRow("SELECT amount FROM ui_cross_scope WHERE id=1").Scan(&value); err != nil || value != expected {
			t.Fatalf("single-write scope on %s: %d, %v", id, value, err)
		}
	}
	setSQL("UPDATE ui_cross_scope SET amount=amount+10 WHERE id=1;")
	run(chromedp.KeyEvent(kb.Enter, chromedp.KeyModifiers(input.ModifierCtrl)))
	wait(`document.querySelector('#db-query-summary').textContent.includes('2/2') && document.querySelector('#db-query-summary').textContent.includes('2 affected')`, "Ctrl+Enter executes current write on checked databases")
	setSQL("ALTER TABLE ui_cross_scope ADD only_west TEXT;")
	run(chromedp.Click(`#db-execute`, chromedp.ByQuery))
	wait(`document.querySelector('#db-query-summary').textContent.includes('1/1') && !document.querySelector('#db-run-query').disabled`, "single-database toolbar supports DDL with multiple checked targets")
	setSQL("UPDATE ui_cross_scope SET only_west='done' WHERE id=1;")
	run(chromedp.Click(`.db-statement-action[data-scope="multi"]`, chromedp.ByQuery))
	wait(`document.querySelector('#db-query-summary').textContent.includes('1/2') && document.querySelector('#db-query-summary').textContent.includes('successful changes remain applied') && document.querySelector('#db-query-grid').textContent.includes('Failed') && document.querySelector('#db-query-grid').textContent.includes('Completed')`, "partial cross-database write reports each destination")
	westPool, _ := databases.pool(context.Background(), databaseTarget{ID: "west", Database: "main"})
	var completedWrite string
	if err := westPool.db.QueryRow("SELECT only_west FROM ui_cross_scope WHERE id=1").Scan(&completedWrite); err != nil || completedWrite != "done" {
		t.Fatalf("successful partial write lost: %q, %v", completedWrite, err)
	}
	if path := os.Getenv("LOG_AGENT_UI_EXECUTION_SCREENSHOT"); path != "" {
		var png []byte
		run(chromedp.FullScreenshot(&png, 100))
		if err := os.WriteFile(path, png, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	setSQL("DELETE FROM ui_cross_scope WHERE id=1;")
	run(chromedp.Click(`#db-run-query`, chromedp.ByQuery))
	wait(`document.querySelector('#db-query-summary').textContent.includes('2/2') && document.querySelector('#db-query-summary').textContent.includes('2 affected')`, "main SQL action executes multi-database delete")
	setSQL("DROP TABLE ui_cross_scope;")
	run(chromedp.Click(`.db-statement-action[data-scope="multi"]`, chromedp.ByQuery))
	wait(`document.querySelector('#db-query-summary').textContent.includes('2/2') && !document.querySelector('#db-run-query').disabled`, "multi-database drop")
	setSQL("SELECT * FROM orders ORDER BY id")
	run(chromedp.Click(`[data-db-database='["east","main"]']`, chromedp.ByQuery))
	wait(`!!document.querySelector('[data-db-table]')`, "table tree")
	run(chromedp.Click(`[data-db-table='["east","main"]']`, chromedp.ByQuery))
	wait(`document.querySelectorAll('#db-data-grid .tabulator-row').length===2`, "editable table")
	run(chromedp.Evaluate(`(() => { const s=document.querySelector('#db-page-size');s.add(new Option('1','1'));s.value='1';s.dispatchEvent(new Event('change', {bubbles:true})); })()`, nil))
	wait(`document.querySelectorAll('#db-data-grid .tabulator-row').length===1 && !document.querySelector('#db-page-last').disabled`, "single-row pages")
	run(chromedp.Evaluate(`window.__firstPageId=document.querySelector('#db-data-grid .tabulator-row [tabulator-field="v0"]').textContent`, nil), chromedp.Click(`#db-page-last`, chromedp.ByQuery))
	wait(`document.querySelector('#db-data-grid .tabulator-row [tabulator-field="v0"]').textContent!==window.__firstPageId && document.querySelector('#db-page-label').textContent.includes('2') && document.querySelector('#db-page-last').disabled`, "last page")
	run(chromedp.Evaluate(`(() => { const s=document.querySelector('#db-page-size');s.value='100';s.dispatchEvent(new Event('change', {bubbles:true})); })()`, nil))
	wait(`document.querySelectorAll('#db-data-grid .tabulator-row').length===2`, "page size restored")
	run(chromedp.DoubleClick(`#db-data-grid .tabulator-row:first-child [tabulator-field="v2"]`, chromedp.ByQuery), chromedp.WaitVisible(`#db-data-grid .tabulator-cell.tabulator-editing input`, chromedp.ByQuery), chromedp.ActionFunc(func(ctx context.Context) error { return input.InsertText("browser edit").Do(ctx) }))
	run(chromedp.KeyEvent(kb.Enter))
	wait(`!document.querySelector('#db-save-rows').disabled`, "pending edit")
	run(chromedp.Click(`#db-save-rows`, chromedp.ByQuery))
	wait(`document.querySelector('#db-status').textContent.includes('Committed 1')`, "committed edit")
	pool, _ := databases.pool(context.Background(), a)
	var note string
	if err := pool.db.QueryRow("SELECT note FROM orders WHERE id=1").Scan(&note); err != nil || note != "browser edit" {
		t.Fatalf("saved value = %q (%v)", note, err)
	}
	run(chromedp.Click(`#db-add-row`, chromedp.ByQuery))
	wait(`document.querySelectorAll('#db-data-grid .tabulator-row').length===3`, "new row")
	run(chromedp.DoubleClick(`#db-data-grid .tabulator-row:nth-child(3) [tabulator-field="v0"]`, chromedp.ByQuery), chromedp.WaitVisible(`#db-data-grid .tabulator-cell.tabulator-editing input`, chromedp.ByQuery), chromedp.ActionFunc(func(ctx context.Context) error { return input.InsertText("3").Do(ctx) }), chromedp.KeyEvent(kb.Enter), chromedp.Click(`#db-save-rows`, chromedp.ByQuery))
	wait(`document.querySelector('#db-status').textContent.includes('Committed 1') && !document.querySelector('#db-panel-data').inert && document.querySelectorAll('#db-data-grid .tabulator-row').length===3 && document.querySelector('#db-data-grid .tabulator-row:nth-child(2) [tabulator-field="v0"]').textContent==='3'`, "insert saved")
	run(chromedp.Click(`#db-data-grid .tabulator-row:nth-child(2) [tabulator-field="__select"]`, chromedp.ByQuery), chromedp.Click(`#db-delete-row`, chromedp.ByQuery))
	wait(`!document.querySelector('#db-save-rows').disabled`, "pending row deletion")
	run(chromedp.Click(`#db-save-rows`, chromedp.ByQuery))
	wait(`document.querySelectorAll('#db-data-grid .tabulator-row').length===2`, "row deletion committed")
	run(chromedp.Click(`[data-db-tab="structure"]`, chromedp.ByQuery), chromedp.Click(`#db-create-table`, chromedp.ByQuery), chromedp.WaitVisible(`#db-schema-modal`, chromedp.ByQuery), chromedp.SendKeys(`#db-schema-fields [name="objectName"]`, "ui_created", chromedp.ByQuery), chromedp.Click(`#db-schema-form button[type="submit"]`, chromedp.ByQuery))
	wait(`!document.querySelector('#db-apply-schema').disabled && document.querySelector('#db-schema-preview').textContent.includes('CREATE TABLE')`, "DDL preview")
	run(chromedp.Click(`#db-apply-schema`, chromedp.ByQuery))
	wait(`document.querySelector('#db-schema-modal').classList.contains('hidden') && document.querySelector('#db-object-title').textContent.includes('ui_created')`, "table created through designer")
	var count int
	if err := pool.db.QueryRow("SELECT count(*) FROM ui_created").Scan(&count); err != nil {
		t.Fatal(err)
	}
	run(chromedp.Click(`#db-add-column`, chromedp.ByQuery), chromedp.WaitVisible(`#db-schema-modal`, chromedp.ByQuery), chromedp.SendKeys(`[data-column-name]`, "amount", chromedp.ByQuery), chromedp.Click(`#db-schema-form button[type="submit"]`, chromedp.ByQuery))
	wait(`!document.querySelector('#db-apply-schema').disabled`, "add-column preview")
	run(chromedp.Click(`#db-apply-schema`, chromedp.ByQuery))
	wait(`document.querySelector('#db-schema-modal').classList.contains('hidden') && document.querySelector('#db-structure-grid').textContent.includes('amount')`, "added column")
	run(chromedp.Click(`#db-create-index`, chromedp.ByQuery), chromedp.WaitVisible(`#db-schema-modal`, chromedp.ByQuery), chromedp.SendKeys(`#db-schema-fields [name="objectName"]`, "idx_ui_name", chromedp.ByQuery), chromedp.Evaluate(`document.querySelector('[name="indexColumns"]').value='name'`, nil), chromedp.Click(`#db-schema-form button[type="submit"]`, chromedp.ByQuery))
	wait(`!document.querySelector('#db-apply-schema').disabled`, "create-index preview")
	run(chromedp.Click(`#db-apply-schema`, chromedp.ByQuery))
	wait(`document.querySelector('#db-schema-modal').classList.contains('hidden') && document.querySelector('#db-index-grid').textContent.includes('idx_ui_name')`, "index in designer")
	run(chromedp.Click(`#db-add-sql-tab`, chromedp.ByQuery))
	setSQL("SELECT name FROM ui_created")
	run(chromedp.Click(`#db-run-query`, chromedp.ByQuery))
	wait(`document.querySelector('#db-query-summary').textContent.includes('1/2') && document.querySelector('#db-query-summary .error') !== null`, "visible partial-source failure")
	setSQL("WITH RECURSIVE n(x) AS (VALUES(0) UNION ALL SELECT x+1 FROM n WHERE x<100000000) SELECT sum(x) FROM n")
	run(chromedp.Click(`#db-run-query`, chromedp.ByQuery))
	wait(`!document.querySelector('#db-cancel-query').disabled`, "running query")
	wait(`document.querySelectorAll('.db-statement-action:enabled').length===0`, "statement actions disabled while querying")
	run(chromedp.Click(`#db-cancel-query`, chromedp.ByQuery))
	wait(`document.querySelector('#db-query-summary').textContent.includes('cancelled') && !document.querySelector('#db-run-query').disabled`, "cancelled query")
	setSQL("SELECT * FROM orders ORDER BY id")
	run(chromedp.Click(`#db-run-query`, chromedp.ByQuery))
	wait(`document.querySelectorAll('#db-query-grid .tabulator-row').length===4`, "query after schema changes")
	// The right rail contains templates only. Table and column pickers are
	// anchored to editable marks inside the SQL document.
	run(chromedp.Evaluate(`document.querySelector('#db-single-target').value='["east","main"]';document.querySelector('#db-single-target').dispatchEvent(new Event('change', {bubbles:true}))`, nil))
	run(chromedp.Evaluate(`window.__templateQueries=0;const templateFetch=window.fetch;window.fetch=(url,...args)=>{if(String(url).includes('/api/database/query'))window.__templateQueries++;return templateFetch(url,...args);}`, nil), chromedp.Click(`#db-toggle-templates`, chromedp.ByQuery))
	wait(`document.querySelectorAll('#db-template-panel [data-db-template]').length===9 && !document.querySelector('#db-template-panel select, #db-template-panel input')`, "templates-only right rail")
	var floatingTemplates bool
	run(chromedp.Evaluate(`(() => { const p=document.querySelector('#db-template-panel'), e=document.querySelector('#db-sql-editor'), before=e.getBoundingClientRect().width;document.querySelector('#db-close-templates').click();const after=e.getBoundingClientRect().width;document.querySelector('#db-toggle-templates').click();return getComputedStyle(p).position==='fixed' && !p.closest('#db-panel-sql') && Math.abs(before-after)<1 && p.getBoundingClientRect().bottom>e.getBoundingClientRect().bottom; })()`, &floatingTemplates))
	if !floatingTemplates {
		t.Fatal("template overlay changed the editor width or was constrained by the SQL pane")
	}
	setSQL("SELECT " + strings.Repeat(`"a_very_long_selected_column", `, 35) + `"last_column" FROM orders;`)
	wait(`(() => {const line=document.querySelector('#db-sql-editor .cm-line'), scroller=document.querySelector('#db-sql-editor .cm-scroller');return line.getBoundingClientRect().height>parseFloat(getComputedStyle(line).lineHeight)*2 && scroller.scrollWidth<=scroller.clientWidth+1;})()`, "long SQL wraps without hidden horizontal content")
	run(chromedp.Click(`[data-db-template="select"]`, chromedp.ByQuery), chromedp.Click(`[data-db-slot-kind="table"]`, chromedp.ByQuery))
	wait(`!!document.querySelector('#db-inline-table') && document.querySelector('#db-inline-table').options.length>=2 && !document.querySelector('#db-inline-apply').disabled`, "inline table picker")
	chooseInlineTable := func(name string) {
		t.Helper()
		value, _ := json.Marshal("." + name)
		run(chromedp.Evaluate(`(() => { const s=document.querySelector('#db-inline-table');s.value=Array.from(s.options).find(o=>o.textContent.endsWith(`+string(value)+`)).value;s.dispatchEvent(new Event('change', {bubbles:true})); })()`, nil))
		wait(`!document.querySelector('#db-inline-apply').disabled`, "inline table metadata")
		run(chromedp.Click(`#db-inline-apply`, chromedp.ByQuery))
	}
	chooseInlineTable("orders")
	run(chromedp.Click(`[data-db-slot-kind="columns"]`, chromedp.ByQuery), chromedp.Click(`#db-inline-none`, chromedp.ByQuery), chromedp.Click(`#db-inline-body [name="inline-column"][value="id"]`, chromedp.ByQuery), chromedp.Click(`#db-inline-apply`, chromedp.ByQuery))
	wait(`document.querySelector('#db-sql-editor .cm-content').textContent.includes('SELECT "id"') && !document.querySelector('#db-sql-editor .cm-content').textContent.includes('amount')`, "selected fields filled inside SQL")
	for _, template := range []struct{ id, fragment string }{{"where", "IS NOT NULL"}, {"order", "ORDER BY"}, {"count", "COUNT(*)"}, {"distinct", "DISTINCT"}, {"group", "GROUP BY"}, {"insert", "INSERT INTO"}, {"update", "UPDATE"}, {"delete", "DELETE FROM"}} {
		run(chromedp.Click(`[data-db-template="`+template.id+`"]`, chromedp.ByQuery))
		fragment, _ := json.Marshal(template.fragment)
		wait(`document.querySelector('#db-sql-editor .cm-content').textContent.includes(`+string(fragment)+`)`, template.id+" template")
	}
	// Templates add a new statement on blank lines and replace the occupied line.
	run(chromedp.Click(`[data-db-template="where"]`, chromedp.ByQuery), chromedp.Evaluate(`window.__firstTemplateSQL=document.querySelector('#db-sql-editor .cm-line').textContent`, nil), chromedp.Focus(`#db-sql-editor .cm-content`, chromedp.ByQuery), chromedp.KeyEvent(kb.Control+kb.End), chromedp.ActionFunc(func(ctx context.Context) error { return input.InsertText("\n").Do(ctx) }), chromedp.Click(`[data-db-template="order"]`, chromedp.ByQuery))
	wait(`(() => {const lines=[...document.querySelectorAll('#db-sql-editor .cm-line')].filter(line=>line.textContent.trim());return lines.length===2 && lines[0].textContent===window.__firstTemplateSQL && lines[1].textContent.includes('ORDER BY');})()`, "blank line inserts a second template")
	run(chromedp.Click(`[data-db-template="count"]`, chromedp.ByQuery))
	wait(`(() => {const lines=[...document.querySelectorAll('#db-sql-editor .cm-line')].filter(line=>line.textContent.trim());return lines.length===2 && lines[0].textContent===window.__firstTemplateSQL && lines[1].textContent.includes('COUNT(*)') && !lines[1].textContent.includes('ORDER BY');})()`, "occupied line replaces only its template")
	run(chromedp.Click(`#db-sql-editor .cm-line:nth-child(2) [data-db-slot-kind="table"]`, chromedp.ByQuery))
	wait(`!!document.querySelector('#db-inline-table') && !document.querySelector('#db-inline-apply').disabled`, "second statement table picker")
	chooseInlineTable("orders")
	wait(`document.querySelector('#db-sql-editor .cm-line').textContent===window.__firstTemplateSQL`, "editing the second statement preserves the first")
	setSQL("SELECT 1;")
	var noAutomaticQuery bool
	run(chromedp.Evaluate(`window.__templateQueries===0 && document.querySelector('#db-target-select').dataset.count==='2'`, &noAutomaticQuery))
	if !noAutomaticQuery {
		t.Fatal("filling templates executed SQL or changed query targets")
	}
	run(chromedp.Click(`[data-db-template="where"]`, chromedp.ByQuery), chromedp.Click(`[data-db-slot-kind="table"]`, chromedp.ByQuery))
	wait(`!!document.querySelector('#db-inline-source')`, "inline schema source")
	run(chromedp.Evaluate(`document.querySelector('#db-inline-source').value='["west","main"]';document.querySelector('#db-inline-source').dispatchEvent(new Event('change', {bubbles:true}))`, nil))
	wait(`!document.querySelector('#db-inline-apply').disabled`, "second database metadata")
	run(chromedp.Click(`#db-inline-apply`, chromedp.ByQuery), chromedp.Click(`[data-db-slot-kind="field"]`, chromedp.ByQuery))
	wait(`document.querySelectorAll('#db-inline-body [name="inline-column"]').length===4`, "inline columns from the second database")
	run(chromedp.Click(`#db-inline-body [value="currency"]`, chromedp.ByQuery), chromedp.Click(`#db-inline-apply`, chromedp.ByQuery))
	wait(`document.querySelector('#db-sql-editor .cm-content').textContent.includes('WHERE "currency" IS NOT NULL')`, "inline filter field")
	chooseOperator := func(operator string) {
		t.Helper()
		value, _ := json.Marshal(operator)
		run(chromedp.Click(`[data-db-slot-kind="operator"]`, chromedp.ByQuery))
		wait(`!!document.querySelector('#db-inline-operator')`, "inline comparison operator picker")
		run(chromedp.Evaluate(`document.querySelector('#db-inline-operator').value=`+string(value), nil), chromedp.Click(`#db-inline-apply`, chromedp.ByQuery))
	}
	chooseOperator("=")
	run(chromedp.Click(`[data-db-slot-kind="filterValue"]`, chromedp.ByQuery), chromedp.ActionFunc(func(ctx context.Context) error { return input.InsertText("100").Do(ctx) }))
	chooseOperator(">=")
	wait(`document.querySelector('#db-sql-editor .cm-content').textContent.includes('"currency" >= 100')`, "operator changes retain a manually edited argument")
	chooseOperator("IN")
	wait(`document.querySelector('#db-sql-editor .cm-content').textContent.includes('"currency" IN (NULL)')`, "list operator creates an editable SQL argument")
	chooseOperator("BETWEEN")
	wait(`document.querySelector('#db-sql-editor .cm-content').textContent.includes('BETWEEN NULL AND NULL')`, "range operator arguments")
	chooseOperator("IS NULL")
	wait(`!document.querySelector('[data-db-slot-kind="filterValue"]') && document.querySelector('#db-sql-editor .cm-content').textContent.includes('"currency" IS NULL') && !document.querySelector('#db-sql-editor .cm-content').textContent.includes('AND NULL')`, "null operator removes its argument")
	chooseOperatorValue := func(operator, field, value string) {
		t.Helper()
		op, _ := json.Marshal(operator)
		id, _ := json.Marshal(field)
		text, _ := json.Marshal(value)
		run(chromedp.Click(`[data-db-slot-kind="operator"]`, chromedp.ByQuery))
		wait(`!!document.querySelector('#db-inline-operator')`, "operator picker with value input")
		run(chromedp.Evaluate(`(() => { const s=document.querySelector('#db-inline-operator');s.value=`+string(op)+`;s.dispatchEvent(new Event('change', {bubbles:true}));document.querySelector(`+string(id)+`).value=`+string(text)+`; })()`, nil), chromedp.Click(`#db-inline-apply`, chromedp.ByQuery))
	}
	chooseOperatorValue("LIKE", "#db-inline-value", "%ab'c%")
	wait(`document.querySelector('#db-sql-editor .cm-content').textContent.includes(`+"`\"currency\" LIKE '%ab''c%'`"+`)`, "operator picker fills a comparison value")
	chooseOperatorValue("IN", "#db-inline-list", "a, b")
	wait(`document.querySelector('#db-sql-editor .cm-content').textContent.includes(`+"`IN ('a', 'b')`"+`)`, "operator picker fills a value list")
	// A copied statement keeps selectable slots, and its operator/value picker
	// edits only the pasted copy.
	run(chromedp.Evaluate(`window.__tabsBefore=document.querySelectorAll('#db-sql-tabs .db-session-tab').length;window.newDatabaseQuery()`, nil))
	wait(`!!document.querySelector('[data-db-template="where"]') && document.querySelectorAll('#db-sql-tabs .db-session-tab').length===window.__tabsBefore+1`, "paste test new tab")
	run(chromedp.Click(`[data-db-template="where"]`, chromedp.ByQuery))
	wait(`!!document.querySelector('[data-db-slot-kind="table"]')`, "paste test where template")
	run(chromedp.Click(`[data-db-slot-kind="table"]`, chromedp.ByQuery))
	wait(`!!document.querySelector('#db-inline-source')`, "paste test source")
	run(chromedp.Evaluate(`document.querySelector('#db-inline-source').value='["west","main"]';document.querySelector('#db-inline-source').dispatchEvent(new Event('change', {bubbles:true}))`, nil))
	wait(`!document.querySelector('#db-inline-apply').disabled`, "paste test metadata")
	run(chromedp.Click(`#db-inline-apply`, chromedp.ByQuery), chromedp.Click(`[data-db-slot-kind="field"]`, chromedp.ByQuery))
	wait(`document.querySelectorAll('#db-inline-body [name="inline-column"]').length===4`, "paste test columns")
	run(chromedp.Click(`#db-inline-body [value="currency"]`, chromedp.ByQuery), chromedp.Click(`#db-inline-apply`, chromedp.ByQuery))
	wait(`document.querySelector('#db-sql-editor .cm-content').textContent.includes('"currency" IS NOT NULL')`, "paste test field")
	run(chromedp.Click(`[data-db-slot-kind="operator"]`, chromedp.ByQuery))
	wait(`!!document.querySelector('#db-inline-operator')`, "paste test operator picker")
	run(chromedp.Evaluate(`(() => { const s=document.querySelector('#db-inline-operator');s.value='=';s.dispatchEvent(new Event('change', {bubbles:true}));document.querySelector('#db-inline-value').value='5'; })()`, nil), chromedp.Click(`#db-inline-apply`, chromedp.ByQuery))
	wait(`document.querySelector('#db-sql-editor .cm-content').textContent.includes("\"currency\" = '5'") && !!document.querySelector('#db-sql-editor [data-db-slot-kind="filterValue"]')`, "paste test comparison value slot")
	run(chromedp.Evaluate(`(() => {
		const content = document.querySelector('#db-sql-editor .cm-content'), data = new DataTransfer();
		content.focus();
		const line = Array.from(content.querySelectorAll('.cm-line')).find(item => item.textContent.includes('currency'));
		const range = document.createRange(); range.selectNodeContents(line);
		const selection = getSelection(); selection.removeAllRanges(); selection.addRange(range);
		window.__pasteData = data;
	})()`, nil), chromedp.Sleep(200*time.Millisecond))
	run(chromedp.ActionFunc(func(ctx context.Context) error { return input.DispatchKeyEvent(input.KeyDown).WithKey("c").WithModifiers(input.ModifierCtrl).WithCommands([]string{"copy"}).Do(ctx) }))
	run(chromedp.Evaluate(`(() => {
		const content = document.querySelector('#db-sql-editor .cm-content');
		const last = content.querySelectorAll('.cm-line'); const end = last[last.length - 1];
		const range = document.createRange(); range.selectNodeContents(end); range.collapse(false);
		const selection = getSelection(); selection.removeAllRanges(); selection.addRange(range);
	})()`, nil), chromedp.Sleep(200*time.Millisecond))
	run(chromedp.ActionFunc(func(ctx context.Context) error { return input.DispatchKeyEvent(input.KeyDown).WithKey("v").WithModifiers(input.ModifierCtrl).WithCommands([]string{"paste"}).Do(ctx) }))
	wait(`document.querySelectorAll('#db-sql-editor [data-db-slot-kind="operator"]').length===2 && document.querySelectorAll('#db-sql-editor [data-db-slot-kind="filterValue"]').length===2`, "pasted statement keeps its slots")
	run(chromedp.Evaluate(`Array.from(document.querySelectorAll('#db-sql-editor [data-db-slot-kind="operator"]')).at(-1).click()`, nil))
	wait(`!!document.querySelector('#db-inline-operator')`, "pasted operator picker")
	run(chromedp.Evaluate(`(() => { const s=document.querySelector('#db-inline-operator');s.value='LIKE';s.dispatchEvent(new Event('change', {bubbles:true}));document.querySelector('#db-inline-value').value='zz'; })()`, nil), chromedp.Click(`#db-inline-apply`, chromedp.ByQuery))
	wait(`document.querySelector('#db-sql-editor .cm-content').textContent.includes(`+"`\"currency\" LIKE 'zz'`"+`) && document.querySelector('#db-sql-editor .cm-content').textContent.includes("\"currency\" = '5'")`, "pasted statement picker edits only the copy")
	run(chromedp.Evaluate(`window.confirm=()=>true;document.querySelector('#db-sql-tabs .db-session-tab.active [data-db-close-sql]').click()`, nil))
	wait(`document.querySelectorAll('#db-sql-tabs .db-session-tab').length===window.__tabsBefore`, "paste test tab closed")
	if _, err := pool.db.Exec(`CREATE TABLE "odd""table" ("odd""column" TEXT); CREATE VIEW template_view AS SELECT id,note FROM orders`); err != nil {
		t.Fatal(err)
	}
	run(chromedp.Click(`[data-db-template="select"]`, chromedp.ByQuery), chromedp.Click(`[data-db-slot-kind="table"]`, chromedp.ByQuery))
	wait(`!!document.querySelector('#db-inline-source')`, "source for quoted table")
	run(chromedp.Evaluate(`document.querySelector('#db-inline-source').value='["east","main"]';document.querySelector('#db-inline-source').dispatchEvent(new Event('change', {bubbles:true}))`, nil))
	wait(`Array.from(document.querySelector('#db-inline-table').options).some(o=>o.textContent.includes('odd"table'))`, "quoted table choice")
	chooseInlineTable(`odd"table`)
	run(chromedp.Click(`[data-db-slot-kind="columns"]`, chromedp.ByQuery), chromedp.Click(`#db-inline-apply`, chromedp.ByQuery))
	wait(`document.querySelector('#db-sql-editor .cm-content').textContent.includes('SELECT *') && document.querySelector('#db-sql-editor .cm-content').textContent.includes('FROM "odd""table"')`, "inline SQL identifier quoting")
	run(chromedp.Click(`[data-db-slot-kind="table"]`, chromedp.ByQuery))
	wait(`!!document.querySelector('#db-inline-table') && !document.querySelector('#db-inline-apply').disabled`, "return to orders table picker")
	chooseInlineTable("orders")
	run(chromedp.Click(`#db-run-query`, chromedp.ByQuery))
	wait(`document.querySelectorAll('#db-query-grid .tabulator-row').length===4 && document.querySelector('#db-query-summary').textContent.includes('2/2')`, "inline template query merged across databases")
	run(chromedp.Click(`#db-close-templates`, chromedp.ByQuery))
	// Each SQL tab preserves its own editor, query targets and result set.
	var firstSQL string
	run(chromedp.Evaluate(`document.querySelector('#db-sql-tabs .active [data-db-sql]').dataset.dbSql`, &firstSQL), chromedp.Click(`#db-add-sql-tab`, chromedp.ByQuery))
	wait(`document.querySelector('#db-sql-editor .cm-content').textContent.trim()===''`, "new blank SQL tab")
	var secondSQL string
	run(chromedp.Evaluate(`document.querySelector('#db-sql-tabs .active [data-db-sql]').dataset.dbSql`, &secondSQL))
	setSQL("SELECT 'other_tab' AS marker;")
	toggleDatabase("west")
	run(chromedp.Click(`#db-run-query`, chromedp.ByQuery))
	wait(`document.querySelectorAll('#db-query-grid .tabulator-row').length===1 && document.querySelector('#db-query-grid').textContent.includes('other_tab')`, "independent SQL result")
	run(chromedp.Click(`[data-db-sql="`+firstSQL+`"]`, chromedp.ByQuery))
	wait(`document.querySelector('#db-sql-editor .cm-content').textContent.includes('FROM "orders"') && document.querySelector('#db-target-select').dataset.count==='2' && document.querySelectorAll('#db-query-grid .tabulator-row').length===4`, "SQL text targets and result restored")
	run(chromedp.Click(`[data-db-sql="`+secondSQL+`"]`, chromedp.ByQuery))
	wait(`document.querySelector('#db-sql-editor .cm-content').textContent.includes('other_tab') && document.querySelector('#db-target-select').dataset.count==='1'`, "second SQL context restored")
	// Tabs and connections can be reordered by dragging.
	dragTab := `(from, to, after) => { const tabs = [...document.querySelectorAll('#db-sql-tabs .db-session-tab')], find = id => tabs.findIndex(tab => tab.dataset.reorder === id), dt = new DataTransfer(), rect = tabs[find(to)].getBoundingClientRect(), point = { bubbles: true, cancelable: true, dataTransfer: dt, clientX: after ? rect.right - 1 : rect.left + 1, clientY: rect.top + 1 }; tabs[find(from)].dispatchEvent(new DragEvent('dragstart', point)); tabs[find(to)].dispatchEvent(new DragEvent('dragover', point)); tabs[find(to)].dispatchEvent(new DragEvent('drop', point)); }`
	var leadSQL string
	run(chromedp.Evaluate(`document.querySelector('#db-sql-tabs [data-db-sql]').dataset.dbSql`, &leadSQL))
	run(chromedp.Evaluate(`(`+dragTab+`)('`+secondSQL+`', '`+leadSQL+`', false)`, nil))
	wait(`document.querySelector('#db-sql-tabs [data-db-sql]').dataset.dbSql==='`+secondSQL+`'`, "SQL tab dragged to the front")
	run(chromedp.Evaluate(`(`+dragTab+`)('`+secondSQL+`', '`+firstSQL+`', true)`, nil))
	wait(`document.querySelector('#db-sql-tabs [data-db-sql]').dataset.dbSql==='`+leadSQL+`' && [...document.querySelectorAll('#db-sql-tabs [data-db-sql]')].at(-1).dataset.dbSql==='`+secondSQL+`'`, "SQL tab dragged back")
	var reordered bool
	run(chromedp.Evaluate(`(async () => { const ids = async () => (await (await fetch('/api/database/profiles')).json()).map(p => p.id), before = await ids(); if (before.length < 2) return false; const reversed = [...before].reverse(); await fetch('/api/database/profiles', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ order: reversed }) }); const after = await ids(); await fetch('/api/database/profiles', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ order: before }) }); return JSON.stringify(after) === JSON.stringify(reversed) && JSON.stringify(await ids()) === JSON.stringify(before); })()`, &reordered, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) }))
	if !reordered {
		t.Fatal("connection order was not persisted")
	}
	setSQL("WITH RECURSIVE n(x) AS (VALUES(0) UNION ALL SELECT x+1 FROM n WHERE x<3000000) SELECT sum(x) AS background_total FROM n;")
	run(chromedp.Click(`#db-run-query`, chromedp.ByQuery), chromedp.Click(`[data-db-sql="`+firstSQL+`"]`, chromedp.ByQuery))
	wait(`!document.querySelector('#db-run-query').disabled && document.querySelectorAll('#db-query-grid .tabulator-row').length===4`, "another tab remains usable during background query")
	wait(`!document.querySelector('[data-db-sql="`+secondSQL+`"]').textContent.includes('◌')`, "background query completed")
	var retainedResult bool
	run(chromedp.Evaluate(`!document.querySelector('#db-query-grid').textContent.includes('background_total')`, &retainedResult))
	if !retainedResult {
		t.Fatal("background query overwrote the active SQL tab")
	}
	run(chromedp.Click(`[data-db-sql="`+secondSQL+`"]`, chromedp.ByQuery))
	wait(`document.querySelector('#db-query-grid').textContent.includes('background_total') && document.querySelectorAll('#db-query-grid .tabulator-row').length===1`, "background result belongs to its SQL tab")
	run(chromedp.Click(`[data-db-sql="`+firstSQL+`"]`, chromedp.ByQuery))
	// Table tabs retain pending edits until the user saves them.
	run(chromedp.Click(`[data-db-bottom="table-1"]`, chromedp.ByQuery), chromedp.Click(`[data-db-tab="data"]`, chromedp.ByQuery))
	wait(`document.querySelectorAll('#db-data-grid .tabulator-row').length===2 && !document.querySelector('#db-panel-data').classList.contains('hidden')`, "orders table tab")
	run(chromedp.DoubleClick(`#db-data-grid .tabulator-row:first-child [tabulator-field="v2"]`, chromedp.ByQuery), chromedp.WaitVisible(`#db-data-grid .tabulator-cell.tabulator-editing input`, chromedp.ByQuery), chromedp.Evaluate(`document.querySelector('#db-data-grid .tabulator-cell.tabulator-editing input').select()`, nil), chromedp.ActionFunc(func(ctx context.Context) error { return input.InsertText("draft kept").Do(ctx) }), chromedp.KeyEvent(kb.Enter))
	wait(`!document.querySelector('#db-save-rows').disabled`, "pending table edit")
	run(chromedp.Click(`[data-db-bottom="table-2"]`, chromedp.ByQuery), chromedp.Click(`[data-db-tab="data"]`, chromedp.ByQuery), chromedp.Click(`[data-db-bottom="table-1"]`, chromedp.ByQuery))
	wait(`document.querySelector('#db-data-grid').textContent.includes('draft kept') && !document.querySelector('#db-save-rows').disabled`, "table draft restored")
	if err := pool.db.QueryRow("SELECT note FROM orders WHERE id=1").Scan(&note); err != nil || note != "browser edit" {
		t.Fatalf("switching table tabs wrote uncommitted data: %q (%v)", note, err)
	}
	run(chromedp.Evaluate(`window.__originalConfirm=window.confirm;window.confirm=()=>false`, nil), chromedp.Click(`[data-db-close-bottom="table-1"]`, chromedp.ByQuery))
	wait(`!!document.querySelector('[data-db-bottom="table-1"]') && document.querySelector('#db-data-grid').textContent.includes('draft kept')`, "closing an unsaved table can be cancelled")
	run(chromedp.Evaluate(`window.confirm=window.__originalConfirm`, nil), chromedp.Click(`#db-save-rows`, chromedp.ByQuery))
	wait(`document.querySelector('#db-status').textContent.includes('Committed 1') && !document.querySelector('#db-panel-data').inert`, "draft saved on its original table")
	if err := pool.db.QueryRow("SELECT note FROM orders WHERE id=1").Scan(&note); err != nil || note != "draft kept" {
		t.Fatalf("table draft was not committed: %q (%v)", note, err)
	}
	run(chromedp.Click(`#db-add-sql-tab`, chromedp.ByQuery))
	var emptySQL string
	run(chromedp.Evaluate(`document.querySelector('#db-sql-tabs .active [data-db-sql]').dataset.dbSql`, &emptySQL))
	run(chromedp.Click(`[data-db-close-sql="`+emptySQL+`"]`, chromedp.ByQuery))
	wait(`!document.querySelector('[data-db-sql="`+emptySQL+`"]')`, "close blank SQL tab")
	run(chromedp.Click(`[data-db-sql="`+firstSQL+`"]`, chromedp.ByQuery))
	var orderedPanes bool
	run(chromedp.Evaluate(`document.querySelector('#db-sql-tabs').getBoundingClientRect().bottom < document.querySelector('#db-sql-editor').getBoundingClientRect().top && document.querySelector('#db-sql-editor').getBoundingClientRect().bottom <= document.querySelector('#db-table-tabs').getBoundingClientRect().top`, &orderedPanes))
	if !orderedPanes {
		t.Fatal("SQL and table tab strips were not arranged above and below the editor")
	}
	run(chromedp.Evaluate(`openDatabaseConnection()`, nil), chromedp.WaitVisible(`#db-connection-modal`, chromedp.ByQuery))
	fileJSON, _ := json.Marshal(filepath.Join(t.TempDir(), "UI connection #中文.db"))
	run(chromedp.Evaluate(`(() => { const f=document.querySelector('#db-connection-form'); f.elements.engine.value='sqlite'; f.elements.engine.dispatchEvent(new Event('change', {bubbles:true})); f.elements.name.value='UI connection'; f.elements.file.value=`+string(fileJSON)+`; })()`, nil), chromedp.Click(`#db-test-connection`, chromedp.ByQuery))
	wait(`document.querySelector('#db-connection-status').textContent==='Connection successful'`, "new SQLite connection tested")
	run(chromedp.Click(`#db-connection-form button[type="submit"]`, chromedp.ByQuery))
	wait(`document.querySelector('#db-connection-modal').classList.contains('hidden') && document.querySelector('#db-tree').textContent.includes('UI connection')`, "connection created")
	run(chromedp.Evaluate(`Array.from(document.querySelectorAll('.db-tree-row')).find(r=>r.textContent.includes('UI connection')).querySelector('[data-db-edit]').click()`, nil), chromedp.WaitVisible(`#db-connection-modal`, chromedp.ByQuery), chromedp.Evaluate(`document.querySelector('#db-connection-form').elements.name.value='Renamed connection'`, nil), chromedp.Click(`#db-connection-form button[type="submit"]`, chromedp.ByQuery))
	wait(`document.querySelector('#db-connection-modal').classList.contains('hidden') && document.querySelector('#db-tree').textContent.includes('Renamed connection')`, "connection edited")
	var emptyConnection string
	run(chromedp.Evaluate(`Array.from(document.querySelectorAll('[data-db-connect]')).find(b=>b.textContent.includes('Renamed connection')).dataset.dbConnect`, &emptyConnection))
	run(chromedp.Click(`[data-db-database='["`+emptyConnection+`","main"]']`, chromedp.ByQuery), chromedp.Click(`#db-tree-create-table`, chromedp.ByQuery), chromedp.WaitVisible(`#db-schema-modal`, chromedp.ByQuery), chromedp.SendKeys(`#db-schema-fields [name="objectName"]`, "empty_database_table", chromedp.ByQuery), chromedp.Click(`#db-schema-form button[type="submit"]`, chromedp.ByQuery))
	wait(`!document.querySelector('#db-apply-schema').disabled`, "empty database create-table preview")
	run(chromedp.Click(`#db-apply-schema`, chromedp.ByQuery))
	wait(`document.querySelector('#db-schema-modal').classList.contains('hidden') && document.querySelector('#db-object-title').textContent.includes('empty_database_table')`, "table created in the explicitly selected empty database")
	emptyPool, err := databases.pool(context.Background(), databaseTarget{ID: emptyConnection, Database: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if err = emptyPool.db.QueryRow("SELECT count(*) FROM empty_database_table").Scan(&count); err != nil {
		t.Fatal(err)
	}
	run(chromedp.Click(`[data-db-bottom="`+firstSQL+`"][data-kind="result"]`, chromedp.ByQuery), chromedp.Evaluate(`document.querySelector('#db-single-target').value='["east","main"]';document.querySelector('#db-single-target').dispatchEvent(new Event('change', {bubbles:true}))`, nil))
	for _, theme := range []string{"dark", "light"} {
		run(chromedp.Evaluate(`applyTheme('`+theme+`')`, nil))
		var overflow bool
		run(chromedp.Evaluate(`document.documentElement.scrollWidth>innerWidth+1 || document.documentElement.scrollHeight>innerHeight+1`, &overflow))
		if overflow {
			t.Fatalf("%s database workspace overflowed", theme)
		}
	}
	run(chromedp.Click(`#db-toggle-templates`, chromedp.ByQuery))
	for _, size := range [][2]int64{{1366, 768}, {1000, 800}} {
		run(chromedp.EmulateViewport(size[0], size[1]), chromedp.Sleep(100*time.Millisecond))
		var validLayout bool
		run(chromedp.Evaluate(`document.documentElement.scrollWidth<=innerWidth+1 && document.documentElement.scrollHeight<=innerHeight+1 && document.querySelector('#db-sql-editor').getBoundingClientRect().height>=85 && document.querySelector('#db-table-tabs').getBoundingClientRect().bottom<innerHeight-50 && document.querySelector('#db-template-panel').getBoundingClientRect().bottom<=innerHeight-8 && document.querySelector('#db-template-panel').getBoundingClientRect().right<=innerWidth-8`, &validLayout))
		if !validLayout {
			t.Fatalf("database tabs overflowed at %dx%d", size[0], size[1])
		}
	}
	run(chromedp.EmulateViewport(1600, 1000))
	run(chromedp.Click(`#db-target-select`, chromedp.ByQuery))
	for _, size := range [][2]int64{{1366, 768}, {1000, 800}} {
		run(chromedp.EmulateViewport(size[0], size[1]), chromedp.Sleep(100*time.Millisecond))
		var visiblePicker bool
		run(chromedp.Evaluate(`(() => {const r=document.querySelector('#db-target-picker').getBoundingClientRect();return r.left>=8 && r.right<=innerWidth-8 && r.bottom<=innerHeight-8 && !document.querySelector('#db-tree input[type="checkbox"]');})()`, &visiblePicker))
		if !visiblePicker {
			t.Fatalf("target picker was clipped at %dx%d", size[0], size[1])
		}
	}
	run(chromedp.EmulateViewport(1600, 1000))
	if path := os.Getenv("LOG_AGENT_UI_SCREENSHOT"); path != "" {
		run(chromedp.Evaluate(`setLanguage('zh');applyTheme('dark');document.querySelector('#db-template-panel').scrollTop=0`, nil), chromedp.Sleep(250*time.Millisecond))
		var png []byte
		run(chromedp.FullScreenshot(&png, 100))
		if err := os.WriteFile(path, png, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	exceptionMu.Lock()
	defer exceptionMu.Unlock()
	if len(exceptions) > 0 {
		t.Fatalf("JavaScript exceptions: %v", exceptions)
	}
}
