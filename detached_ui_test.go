package main

import (
	"context"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"net/http/httptest"
	"testing"
	"time"
)

func TestE2ENavigationDetach(t *testing.T) {
	browser := firstExistingPath(`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`)
	if browser == "" {
		t.Skip("Chrome or Edge required")
	}
	databaseTestSetup(t)
	web := httptest.NewServer(newHTTPHandler(newServer()))
	defer web.Close()
	allocator, stop := chromedp.NewExecAllocator(context.Background(), append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(browser), chromedp.NoSandbox, chromedp.WindowSize(1440, 950))...)
	defer stop()
	ctx, closeBrowser := chromedp.NewContext(allocator)
	defer closeBrowser()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	run := func(actions ...chromedp.Action) {
		t.Helper()
		if err := chromedp.Run(ctx, actions...); err != nil {
			t.Fatal(err)
		}
	}
	run(chromedp.Navigate(web.URL), chromedp.WaitVisible(`.rail .nav-item[data-view="commands"]`, chromedp.ByQuery), chromedp.Poll(`typeof i18n!=='undefined' && !!i18n.en['nav.detachFailed']`, nil), chromedp.Evaluate(`window.__detached=[];window.__lbOpenView=async(...args)=>window.__detached.push(args)`, nil))
	for _, view := range []string{"overview", "rules", "connections", "commands", "terminal", "database", "redis"} {
		var point struct{ X, Y float64 }
		run(chromedp.Evaluate(`(()=>{const r=document.querySelector('.rail .nav-item[data-view="`+view+`"]').getBoundingClientRect();return {X:r.x+r.width/2,Y:r.y+r.height/2};})()`, &point))
		run(chromedp.MouseEvent(input.MousePressed, point.X, point.Y, chromedp.ButtonLeft),
			chromedp.MouseEvent(input.MouseMoved, point.X+180, point.Y, chromedp.ButtonLeft),
			chromedp.MouseEvent(input.MouseReleased, point.X+180, point.Y, chromedp.ButtonLeft))
	}
	var valid bool
	run(chromedp.Evaluate(`window.__detached.map(args=>args[0]).join(',')==='overview,rules,connections,commands,terminal,database,redis' && !document.querySelector('#nav-detach-preview') && document.querySelector('.nav-item.active').dataset.view==='overview'`, &valid))
	if !valid {
		var detail string
		run(chromedp.Evaluate(`JSON.stringify({opened:window.__detached,active:document.querySelector('.nav-item.active')?.dataset.view,preview:!!document.querySelector('#nav-detach-preview')})`, &detail))
		t.Fatalf("dragging navigation must open the requested page without changing the source page: %s", detail)
	}
	run(chromedp.Click(`.rail .nav-item[data-view="commands"]`, chromedp.ByQuery))
	run(chromedp.Evaluate(`!document.querySelector('#commands-view').classList.contains('hidden') && window.__detached.length===7`, &valid))
	if !valid {
		t.Fatal("normal navigation clicks must continue to work")
	}
	run(chromedp.Navigate(web.URL+"?detached=database"), chromedp.WaitVisible(`#db-sql-editor .cm-editor`, chromedp.ByQuery))
	run(chromedp.Evaluate(`document.querySelector('.nav-item.active').dataset.view==='database'`, &valid))
	if !valid {
		t.Fatal("detached window did not initialize the requested page")
	}
}
