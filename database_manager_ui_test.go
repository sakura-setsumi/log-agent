package main

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestE2EDatabaseManager(t *testing.T) {
	browser := firstExistingPath(`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`)
	if browser == "" {
		t.Skip("Chrome or Edge required")
	}
	databaseTestSetup(t)
	profiles, err := readDatabaseProfiles()
	if err != nil {
		t.Fatal(err)
	}
	profiles = append(profiles, databaseProfile{ID: "saved", Name: "Manager fixture", Engine: "mysql", Host: "localhost", Port: 3306, User: "tester", Password: "fixture-secret"})
	if err := writeJSONFile(databaseProfilesPath(), profiles); err != nil {
		t.Fatal(err)
	}
	web := httptest.NewServer(newHTTPHandler(newServer()))
	defer web.Close()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(browser), chromedp.NoSandbox, chromedp.WindowSize(1200, 900))
	allocator, cancelAllocator := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAllocator()
	ctx, cancelBrowser := chromedp.NewContext(allocator)
	defer cancelBrowser()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	run := func(actions ...chromedp.Action) {
		t.Helper()
		if err := chromedp.Run(ctx, actions...); err != nil {
			t.Fatal(err)
		}
	}
	check := func(expression string) {
		t.Helper()
		var ok bool
		run(chromedp.Evaluate(expression, &ok))
		if !ok {
			t.Fatalf("check failed: %s", expression)
		}
	}
	run(chromedp.Navigate(web.URL), chromedp.WaitVisible(`#menubar`, chromedp.ByQuery), chromedp.Evaluate(`document.querySelector('[data-call="openDatabaseManager"]').click()`, nil), chromedp.WaitVisible(`[data-db-manager-edit="saved"]`, chromedp.ByQuery))
	run(chromedp.Evaluate(`document.querySelector('#db-manager-search').value='tester';document.querySelector('#db-manager-search').dispatchEvent(new Event('input'))`, nil))
	check(`document.querySelectorAll('[data-db-manager-edit]').length===1`)
	run(chromedp.Click(`[data-db-manager-edit="saved"]`, chromedp.ByQuery), chromedp.WaitVisible(`#db-connection-modal`, chromedp.ByQuery))
	check(`document.querySelector('#db-connection-password').type==='password' && document.querySelector('#db-connection-password').value===''`)
	run(chromedp.Click(`#db-toggle-password`, chromedp.ByQuery), chromedp.Poll(`document.querySelector('#db-connection-password').type==='text'`, nil))
	check(`document.querySelector('#db-connection-password').value==='fixture-secret' && document.querySelector('#db-connection-form').dataset.passwordDirty==='false'`)
	run(chromedp.Click(`#db-toggle-password`, chromedp.ByQuery))
	check(`document.querySelector('#db-connection-password').type==='password' && document.querySelector('#db-toggle-password').getAttribute('aria-pressed')==='false'`)
	run(chromedp.Click(`[data-db-close="connection"]`, chromedp.ByQuery))
	check(`document.querySelector('#db-connection-password').value==='' && !document.querySelector('#db-manager-modal').classList.contains('hidden')`)
	run(chromedp.Click(`#db-manager-new`, chromedp.ByQuery), chromedp.WaitVisible(`#db-connection-modal`, chromedp.ByQuery), chromedp.SendKeys(`#db-connection-password`, "new-secret", chromedp.ByQuery), chromedp.Click(`#db-toggle-password`, chromedp.ByQuery))
	check(`document.querySelector('#db-connection-password').type==='text' && document.querySelector('#db-connection-password').value==='new-secret' && document.querySelector('#db-connection-form').dataset.passwordDirty==='true'`)
	run(chromedp.Click(`[data-db-close="connection"]`, chromedp.ByQuery), chromedp.Click(`[data-db-manager-edit="saved"]`, chromedp.ByQuery))
	check(`document.querySelector('#db-connection-password').type==='password' && document.querySelector('#db-connection-password').value===''`)
}
