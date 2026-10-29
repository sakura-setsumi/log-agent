package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

func nodeOrderFixture(t *testing.T) *server {
	t.Helper()
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	resetConfigDirCache(t)
	s := newServer()
	for _, name := range []string{"Alpha", "Beta", "Gamma"} {
		if _, err := s.store.insertNode(name, "http://127.0.0.1:8099/"+name, "HTTP / WebSocket"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.loadNodesFromStore(); err != nil {
		t.Fatal(err)
	}
	return s
}
func nodeOrderRequest(s *server, body, remote string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPut, "/api/nodes", bytes.NewBufferString(body))
	r.RemoteAddr = remote
	w := httptest.NewRecorder()
	s.handleNodes(w, r)
	return w
}
func TestNodeOrderingPersistsAndPreservesState(t *testing.T) {
	s := nodeOrderFixture(t)
	s.nodes[0].Status = "connected"
	s.nodes[0].Containers = []containerInfo{{ID: "keep", Name: "keep"}}
	original := append([]Node{}, s.nodes...)
	before, err := s.store.listNodes()
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{original[2].ID, original[0].ID, original[1].ID}
	body, _ := json.Marshal(map[string]any{"order": ids})
	response := nodeOrderRequest(s, string(body), "127.0.0.1:1234")
	if response.Code != 200 {
		t.Fatalf("reorder: %d %s", response.Code, response.Body)
	}
	for i, index := range []int{2, 0, 1} {
		if !reflect.DeepEqual(s.nodes[i], original[index]) {
			t.Fatalf("runtime state changed for node %s", ids[i])
		}
	}
	stored, err := newConfigStore().listNodes()
	if err != nil {
		t.Fatal(err)
	}
	for i, index := range []int{2, 0, 1} {
		if stored[i] != before[index] {
			t.Fatalf("stored node changed: %+v", stored)
		}
	}
	// Later edits/additions must not revert order to numeric ID order.
	if err := s.store.updateNode(before[0].ID, "Edited", before[0].Address, before[0].Style); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.insertNode("New", "http://localhost:9099", "HTTP / WebSocket"); err != nil {
		t.Fatal(err)
	}
	stored, err = newConfigStore().listNodes()
	if err != nil || len(stored) != 4 || stored[0].ID != before[2].ID || stored[1].ID != before[0].ID || stored[3].Name != "New" {
		t.Fatalf("order after edits: %+v %v", stored, err)
	}
	// Duplicate/unknown IDs are ignored; omitted nodes retain relative order.
	response = nodeOrderRequest(s, `{"order":["`+original[1].ID+`","missing","`+original[1].ID+`"]}`, "127.0.0.1:1234")
	if response.Code != 200 || s.nodes[0].ID != original[1].ID || len(s.nodes) != 3 {
		t.Fatalf("stale order response: %d %s", response.Code, response.Body)
	}
	if got := nodeOrderRequest(s, `{"order":"bad"}`, "127.0.0.1:1234"); got.Code != 400 {
		t.Fatalf("malformed order status %d", got.Code)
	}
	if got := nodeOrderRequest(s, string(body), "198.51.100.2:1234"); got.Code != 403 {
		t.Fatalf("remote reorder status %d", got.Code)
	}
}
func TestNodeOrderingSaveFailureKeepsPreviousOrder(t *testing.T) {
	s := nodeOrderFixture(t)
	previous := append([]Node{}, s.nodes...)
	stored, _ := s.store.listNodes()
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	s.store.dir = blocked
	response := nodeOrderRequest(s, `{"order":["`+s.nodes[2].ID+`"]}`, "127.0.0.1:1234")
	if response.Code != 500 {
		t.Fatalf("save error status %d", response.Code)
	}
	after, _ := s.store.listNodes()
	if !reflect.DeepEqual(previous, s.nodes) || !reflect.DeepEqual(stored, after) {
		t.Fatal("failed save changed runtime/store order")
	}
	disk, err := newConfigStore().listNodes()
	if err != nil || !reflect.DeepEqual(stored, disk) {
		t.Fatalf("failed save changed disk: %v", err)
	}
}
func TestE2ENodeManagerOrdering(t *testing.T) {
	browser := firstExistingPath(`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`)
	if browser == "" {
		t.Skip("Chrome or Edge required")
	}
	s := nodeOrderFixture(t)
	first, last := s.nodes[0].ID, s.nodes[2].ID
	web := httptest.NewServer(newHTTPHandler(s))
	defer web.Close()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(browser), chromedp.NoSandbox, chromedp.WindowSize(1200, 900))
	allocator, closeAllocator := chromedp.NewExecAllocator(context.Background(), opts...)
	defer closeAllocator()
	ctx, closeBrowser := chromedp.NewContext(allocator, chromedp.WithErrorf(func(string, ...any) {}))
	defer closeBrowser()
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
		run(chromedp.Poll(expression, nil, chromedp.WithPollingTimeout(10*time.Second)))
	}
	run(chromedp.Navigate(web.URL), chromedp.WaitVisible(`#menubar`, chromedp.ByQuery))
	wait(`nodes.length===3 && state.storageMode==='file'`)
	run(chromedp.Evaluate(`openNodeManager();state.selectedNodes=[nodes[0].id];document.querySelector('#add-node-form').elements.name.value='Unsaved draft'`, nil))
	var source, target struct{ X, Y float64 }
	run(chromedp.Evaluate(`(()=>{const r=document.querySelector('[data-manage-node-id="`+last+`"] .connection-order-handle').getBoundingClientRect();return {X:r.left+r.width/2,Y:r.top+r.height/2}})()`, &source), chromedp.Evaluate(`(()=>{const r=document.querySelector('[data-manage-node-id="`+first+`"]').getBoundingClientRect();return {X:r.left+r.width/2,Y:r.top+r.height/4}})()`, &target))
	run(chromedp.ActionFunc(func(ctx context.Context) error {
		if err := input.DispatchMouseEvent(input.MouseMoved, source.X, source.Y).Do(ctx); err != nil {
			return err
		}
		if err := input.DispatchMouseEvent(input.MousePressed, source.X, source.Y).WithButton(input.Left).WithClickCount(1).Do(ctx); err != nil {
			return err
		}
		for step := 1; step <= 12; step++ {
			f := float64(step) / 12
			if err := input.DispatchMouseEvent(input.MouseMoved, source.X+(target.X-source.X)*f, source.Y+(target.Y-source.Y)*f).WithButton(input.Left).WithButtons(1).Do(ctx); err != nil {
				return err
			}
		}
		return input.DispatchMouseEvent(input.MouseReleased, target.X, target.Y).WithButton(input.Left).WithClickCount(1).Do(ctx)
	}))
	wait(`!document.querySelector('#node-manager-list').hasAttribute('aria-busy') && nodes[0].id==='` + last + `' && document.querySelector('[data-manage-node-id]').dataset.manageNodeId==='` + last + `' && document.querySelector('#node-list .node-item').dataset.nodeId==='` + last + `'`)
	wait(`document.querySelector('#add-node-form').elements.name.value==='Unsaved draft' && document.querySelector('#add-node-modal').dataset.nodeId==='` + first + `' && state.selectedNodes[0]==='` + first + `'`)
	run(chromedp.Evaluate(`syncGoBackend({incremental:true})`, nil))
	wait(`nodes[0].id==='` + last + `' && document.querySelector('#add-node-form').elements.name.value==='Unsaved draft'`)
	run(chromedp.Reload(), chromedp.WaitVisible(`#menubar`, chromedp.ByQuery))
	wait(`nodes[0]?.id==='` + last + `'`)
	run(chromedp.Evaluate(`openNodeManager();window.__nodeFetch=window.fetch;window.fetch=(url,options)=>url==='/api/nodes'&&options?.method==='PUT'?Promise.resolve(new Response(JSON.stringify({error:'fixture save failed'}),{status:500})):window.__nodeFetch(url,options);reorderNodes(nodes[0].id,nodes[2].id,true)`, nil))
	wait(`document.querySelector('#toast').textContent.includes('fixture save failed') && nodes[0].id==='` + last + `'`)
	// Browser-only node configurations use their existing localStorage backend.
	run(chromedp.Evaluate(`window.fetch=window.__nodeFetch;state.paused=true;eventStream?.close();applyStorageMode('browser');reorderNodes(nodes[0].id,nodes[2].id,true)`, nil))
	wait(`loadBrowserNodes()[2]?.id==='` + last + `' && nodes[2].id==='` + last + `'`)
	run(chromedp.Evaluate(`window.__setItem=Storage.prototype.setItem;Storage.prototype.setItem=function(){throw new Error('quota')};reorderNodes(nodes[2].id,nodes[0].id,false)`, nil))
	wait(`nodes[2].id==='` + last + `' && loadBrowserNodes()[2]?.id==='` + last + `'`)
	run(chromedp.Evaluate(`Storage.prototype.setItem=window.__setItem`, nil))
}
