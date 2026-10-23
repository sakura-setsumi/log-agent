package main

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestE2EDatabaseTemplateClickScope(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	run := func(actions ...chromedp.Action) {
		t.Helper()
		if err := chromedp.Run(ctx, actions...); err != nil {
			t.Fatal(err)
		}
	}
	run(chromedp.Navigate(web.URL), chromedp.WaitReady(`body`, chromedp.ByQuery), chromedp.Poll(`!!window.DatabaseUI`, nil))
	var result string
	run(chromedp.Evaluate(`(() => {
      const host=document.createElement('div');host.id='scope-fixture';host.style.height='300px';document.body.append(host);
      const ui=DatabaseUI.createEditor(host,()=>{},()=>{},()=>{},slot=>{window.scopeClicked=slot});
      window.scopeFixture=ui;
      const original='SELECT * FROM orders WHERE id = 220 LIMIT 100;\nSELECT * FROM orders WHERE id = 197;';
      ui.setValue(original);ui.configure('sqlite');ui.recognizeSlots();
      const saved=ui.slotState();
      // Simulate saved templates with duplicate group AND slot IDs after restart.
      saved.slots=saved.slots.map(slot=>({...slot,id:slot.kind,template:{id:'template-1',templateId:'where'}}));
      ui.setSlotState(saved);
      if(new Set(ui.slots().map(slot=>slot.id)).size!==ui.slots().length) return 'duplicate restored slot IDs';
      host.querySelectorAll('[data-db-slot-kind="operator"]')[1].click();
      if(ui.slotText('filterValue')!=='197') return 'read wrong value: '+ui.slotText('filterValue');
      ui.replacePredicate('=','330','value');
      if(ui.value()!==original.replace('197','330')) return 'changed wrong statement: '+ui.value();
      ui.replaceSlots({field:'amount'});
      if(ui.value()!=='SELECT * FROM orders WHERE id = 220 LIMIT 100;\nSELECT * FROM orders WHERE amount = 330;') return 'changed wrong field: '+ui.value();
      const snapshot=ui.snapshot();ui.setValue('SELECT 1;');ui.restore(snapshot);
      host.querySelectorAll('[data-db-slot-kind="operator"]')[1].click();
      if(ui.slotText('filterValue')!=='330') return 'snapshot lost clicked scope';
      ui.setValue("SELECT * FROM orders WHERE id = 1 AND amount >= 2 OR note LIKE 'old';");ui.recognizeSlots();
      host.querySelectorAll('[data-db-slot-kind="operator"]')[2].click();
      if(ui.slotText('filterValue')!=="'old'") return 'read first predicate instead of OR';
      ui.replacePredicate('LIKE',"'new'",'value');
      if(ui.value()!=="SELECT * FROM orders WHERE id = 1 AND amount >= 2 OR note LIKE 'new';") return 'changed wrong OR predicate: '+ui.value();
      host.querySelectorAll('[data-db-slot-kind="field"]')[1].click();ui.replaceSlots({field:'total'});
      if(ui.value()!=="SELECT * FROM orders WHERE id = 1 AND total >= 2 OR note LIKE 'new';") return 'changed wrong AND field: '+ui.value();
      ui.replacePredicate('IN','(3, 4)','value');
      if(ui.value()!=="SELECT * FROM orders WHERE id = 1 AND total IN (3, 4) OR note LIKE 'new';") return 'changed wrong AND value: '+ui.value();
      return 'ok';
    })()`, &result))
	if result != "ok" {
		t.Fatal(result)
	}
}
