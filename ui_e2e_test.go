package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

type assistantMarkdownSafety struct {
	HasCodeBlock   bool `json:"hasCodeBlock"`
	HasRawImage    bool `json:"hasRawImage"`
	HasScriptTag   bool `json:"hasScriptTag"`
	HasTable       bool `json:"hasTable"`
	SafeLink       bool `json:"safeLink"`
	UnsafeLink     bool `json:"unsafeLink"`
	XSSExecuted    bool `json:"xssExecuted"`
	SameBackground bool `json:"sameBackground"`
	HasDivider     bool `json:"hasDivider"`
	HasBlockquote  bool `json:"hasBlockquote"`
	HasDeepHeading bool `json:"hasDeepHeading"`
	HasMixedBold   bool `json:"hasMixedBold"`
}

type logStreamPauseView struct {
	Processed    string `json:"processed"`
	Paused       bool   `json:"paused"`
	NewLogCount  int    `json:"newLogCount"`
	StreamStatus string `json:"streamStatus"`
}

type logStreamBurstView struct {
	Buffered     int `json:"buffered"`
	Pending      int `json:"pending"`
	RenderedRows int `json:"renderedRows"`
	NewestID     int `json:"newestId"`
}

type modalDragDismissalView struct {
	RemainedOpenAfterDrag bool `json:"remainedOpenAfterDrag"`
	ClosedAfterBackdrop   bool `json:"closedAfterBackdrop"`
}

type logOrderView struct {
	FirstID             int    `json:"firstId"`
	LastID              int    `json:"lastId"`
	AtBottom            bool   `json:"atBottom"`
	FlowVisibleAtBottom bool   `json:"flowVisibleAtBottom"`
	FollowDisabled      bool   `json:"followDisabled"`
	FlowHiddenAbove     bool   `json:"flowHiddenAbove"`
	ContainerOrigin     string `json:"containerOrigin"`
	DateTop             string `json:"dateTop"`
	MetaBorderStyle     string `json:"metaBorderStyle"`
	MetaWidth           string `json:"metaWidth"`
	MetaBackground      string `json:"metaBackground"`
	EvenRowBackground   string `json:"evenRowBackground"`
	OddRowBackground    string `json:"oddRowBackground"`
}

type logFlowView struct {
	AnimationName     string `json:"animationName"`
	AnimationDuration string `json:"animationDuration"`
}

func TestE2ELogStreamPauseFreezesAndResumes(t *testing.T) {
	browserPath := firstExistingPath(
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	)
	if browserPath == "" {
		t.Skip("Chrome or Edge is required for UI end-to-end tests")
	}

	now := time.Now()
	first := LogEntry{
		ID: 1, Date: now.Format("2006/01/02"), Time: now.Format("15:04:05"), Timestamp: now.UnixMilli(),
		Level: "info", Node: "E2E Node", Container: "api", Message: "first log", nodeID: "e2e-node",
	}
	s := &server{
		nodes:                 []Node{{ID: "e2e-node", Name: "E2E Node", Status: "connected"}},
		logs:                  []LogEntry{first},
		processed:             1,
		nextLogID:             1,
		historyRange:          "30m",
		rules:                 map[string]bool{"mask": true, "structure": true, "noise": false},
		ruleOrder:             defaultRuleOrder(),
		subscribers:           make(map[chan LogEntry]struct{}),
		containerNames:        make(map[string]map[string]string),
		containerLogs:         make(map[string][]LogEntry),
		historyCoverage:       make(map[string]time.Time),
		historyLoads:          make(map[string]struct{}),
		historyLoadGeneration: make(map[string]uint64),
		streams:               make(map[string]struct{}),
		nodeContexts:          make(map[string]context.Context),
		nodeCancels:           make(map[string]context.CancelFunc),
		settings:              defaultAppSettings(),
	}
	web := httptest.NewServer(newHTTPHandler(s))
	t.Cleanup(web.Close)

	allocatorOptions := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	allocatorOptions = append(allocatorOptions,
		chromedp.ExecPath(browserPath),
		chromedp.Headless,
		chromedp.NoSandbox,
		chromedp.WindowSize(1440, 1000),
	)
	allocatorContext, cancelAllocator := chromedp.NewExecAllocator(context.Background(), allocatorOptions...)
	t.Cleanup(cancelAllocator)
	browserContext, cancelBrowser := chromedp.NewContext(allocatorContext,
		chromedp.WithErrorf(func(string, ...any) {}),
	)
	t.Cleanup(cancelBrowser)

	if err := chromedp.Run(browserContext,
		chromedp.Navigate(web.URL),
		chromedp.WaitVisible(`[data-log-id="1"]`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("load initial log stream: %v", err)
	}
	var flowView logFlowView
	if err := chromedp.Run(browserContext, chromedp.Evaluate(`(() => {
		const style = getComputedStyle(document.querySelector('.logs-panel .stream-footer'), '::after');
		return { animationName: style.animationName, animationDuration: style.animationDuration };
	})()`, &flowView)); err != nil {
		t.Fatalf("read log flow animation: %v", err)
	}
	if flowView.AnimationName != "log-stream-flow" || flowView.AnimationDuration == "0s" {
		t.Fatalf("log flow animation is disabled: %+v", flowView)
	}
	waitForSubscriberCount(t, s, 1)

	if err := chromedp.Run(browserContext,
		chromedp.Click(`#pause-button`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("pause log stream: %v", err)
	}
	waitForSubscriberCount(t, s, 0)

	secondTime := now.Add(time.Second)
	second := LogEntry{
		ID: 2, Date: secondTime.Format("2006/01/02"), Time: secondTime.Format("15:04:05"), Timestamp: secondTime.UnixMilli(),
		Level: "error", Node: "E2E Node", Container: "api", Message: "log received while paused", nodeID: "e2e-node",
	}
	s.mu.Lock()
	s.logs = []LogEntry{second, first}
	s.processed = 2
	s.nextLogID = 2
	s.mu.Unlock()

	// Wait longer than the three-second bootstrap refresh interval. The paused
	// view must remain frozen even if a refresh was already in flight.
	if err := chromedp.Run(browserContext, chromedp.Sleep(3500*time.Millisecond)); err != nil {
		t.Fatalf("wait while paused: %v", err)
	}
	pausedView := readLogStreamPauseView(t, browserContext)
	if pausedView.Processed != "1" || !pausedView.Paused || pausedView.NewLogCount != 0 || pausedView.StreamStatus != "已暂停接收" {
		t.Fatalf("paused view changed while backend received a log: %+v", pausedView)
	}

	if err := chromedp.Run(browserContext,
		chromedp.Click(`#pause-button`, chromedp.ByQuery),
		chromedp.WaitVisible(`[data-log-id="2"]`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("resume and catch up log stream: %v", err)
	}
	resumedView := readLogStreamPauseView(t, browserContext)
	if resumedView.Processed != "2" || resumedView.Paused || resumedView.NewLogCount != 1 {
		t.Fatalf("resumed view did not catch up: %+v", resumedView)
	}
	waitForSubscriberCount(t, s, 1)

	var burstView logStreamBurstView
	if err := chromedp.Run(browserContext,
		chromedp.Evaluate(`(() => {
			const now = Date.now();
			for (let index = 0; index < 2000; index += 1) {
				enqueueStreamLog({
					id: 1000 + index,
					date: '2026/09/14',
					time: '12:00:00',
					timestamp: now + index,
					level: 'info',
					node: 'E2E Node',
					container: 'api',
					message: 'restart burst',
				});
			}
			return true;
		})()`, nil),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.Evaluate(`({
			buffered: state.logs.length,
			pending: pendingStreamLogs.length,
			renderedRows: document.querySelectorAll('#log-stream .log-row').length,
			newestId: state.logs[0]?.id || 0,
		})`, &burstView),
		chromedp.Click(`#pause-button`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("process restart log burst: %v", err)
	}
	if burstView.Buffered != 2002 || burstView.Pending != 0 || burstView.RenderedRows > 200 || burstView.NewestID != 2999 {
		t.Fatalf("log burst was not batched and virtualized: %+v", burstView)
	}
	var pausedStatus string
	if err := chromedp.Run(browserContext, chromedp.Text(`#stream-status`, &pausedStatus, chromedp.ByQuery)); err != nil {
		t.Fatalf("read stream status after burst: %v", err)
	}
	if pausedStatus != "已暂停接收" {
		t.Fatalf("dashboard did not remain interactive after burst, status=%q", pausedStatus)
	}
	waitForSubscriberCount(t, s, 0)

	var modalView modalDragDismissalView
	if err := chromedp.Run(browserContext, chromedp.Evaluate(`(() => {
		const modal = document.querySelector('#app-settings-modal');
		const card = modal.querySelector('.modal-card');
		modal.classList.remove('hidden');
		card.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, pointerId: 1 }));
		modal.dispatchEvent(new PointerEvent('pointerup', { bubbles: true, pointerId: 1 }));
		modal.dispatchEvent(new MouseEvent('click', { bubbles: true }));
		const remainedOpenAfterDrag = !modal.classList.contains('hidden');
		modal.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, pointerId: 2 }));
		modal.dispatchEvent(new PointerEvent('pointerup', { bubbles: true, pointerId: 2 }));
		modal.dispatchEvent(new MouseEvent('click', { bubbles: true }));
		return {
			remainedOpenAfterDrag,
			closedAfterBackdrop: modal.classList.contains('hidden'),
		};
	})()`, &modalView)); err != nil {
		t.Fatalf("exercise modal backdrop dismissal: %v", err)
	}
	if !modalView.RemainedOpenAfterDrag || !modalView.ClosedAfterBackdrop {
		t.Fatalf("unexpected modal backdrop dismissal behavior: %+v", modalView)
	}

	var orderView logOrderView
	if err := chromedp.Run(browserContext, chromedp.Evaluate(`(() => {
		const now = Date.now();
		state.logs = Array.from({ length: 100 }, (_, index) => {
			const id = 100 - index;
			return {
				id,
				date: '2026/09/14',
				time: '12:00:00',
				timestamp: now + id,
				level: 'info',
				node: 'E2E Node',
				container: 'api',
				message: 'ordered log',
			};
		});
		state.query = '';
		state.globalQuery = '';
		state.selectedNodes = [];
		state.selectedContainers = [];
		state.level = 'all';
		state.historyLoading = false;
		followLatestLogs = true;
		renderLogs();
		const stream = document.querySelector('#log-stream');
		const footer = document.querySelector('.logs-panel .stream-footer');
		const ids = Array.from(stream.querySelectorAll('.log-row')).map((row) => Number(row.dataset.logId));
		const atBottom = isLogStreamNearBottom(stream);
		const flowVisibleAtBottom = footer.classList.contains('is-at-latest');
		stream.scrollTop = 0;
		stream.dispatchEvent(new Event('scroll'));
		return {
			firstId: ids[0],
			lastId: ids[ids.length - 1],
			atBottom,
			flowVisibleAtBottom,
			followDisabled: followLatestLogs === false,
			flowHiddenAbove: !footer.classList.contains('is-at-latest'),
			containerOrigin: stream.querySelector('.log-container-origin')?.textContent || '',
			dateTop: getComputedStyle(stream.querySelector('.log-meta')).top,
			metaBorderStyle: getComputedStyle(stream.querySelector('.log-meta')).borderTopStyle,
			metaWidth: getComputedStyle(stream.querySelector('.log-meta')).width,
			metaBackground: getComputedStyle(stream.querySelector('.log-meta')).backgroundColor,
			evenRowBackground: getComputedStyle(stream.querySelector('.log-row-tone-even')).backgroundColor,
			oddRowBackground: getComputedStyle(stream.querySelector('.log-row-tone-odd')).backgroundColor,
		};
	})()`, &orderView)); err != nil {
		t.Fatalf("verify chronological log ordering: %v", err)
	}
	if orderView.FirstID != 1 || orderView.LastID != 100 || !orderView.AtBottom || !orderView.FlowVisibleAtBottom || !orderView.FollowDisabled || !orderView.FlowHiddenAbove || orderView.ContainerOrigin != "E2E Node/api" || orderView.DateTop != "2px" || orderView.MetaBorderStyle != "solid" || orderView.MetaWidth != "130px" || orderView.MetaBackground != "rgb(255, 255, 255)" || orderView.EvenRowBackground != "rgb(245, 245, 245)" || orderView.OddRowBackground != "rgb(239, 239, 240)" {
		t.Fatalf("unexpected chronological log stream behavior: %+v", orderView)
	}
}

func readLogStreamPauseView(t *testing.T, browserContext context.Context) logStreamPauseView {
	t.Helper()
	var view logStreamPauseView
	if err := chromedp.Run(browserContext, chromedp.Evaluate(`({
		processed: document.querySelector('#metric-processed').textContent,
		paused: state.paused,
		newLogCount: document.querySelectorAll('[data-log-id="2"]').length,
		streamStatus: document.querySelector('#stream-status').textContent,
	})`, &view)); err != nil {
		t.Fatalf("read log stream state: %v", err)
	}
	return view
}

func waitForSubscriberCount(t *testing.T, s *server, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.RLock()
		count := len(s.subscribers)
		s.mu.RUnlock()
		if count == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.mu.RLock()
	count := len(s.subscribers)
	s.mu.RUnlock()
	t.Fatalf("subscriber count = %d, want %d", count, want)
}

func TestE2EAssistantInteractions(t *testing.T) {
	browserPath := firstExistingPath(
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	)
	if browserPath == "" {
		t.Skip("Chrome or Edge is required for UI end-to-end tests")
	}

	now := time.Now()
	s := &server{
		nodes: []Node{{ID: "e2e-node", Name: "E2E Node", Status: "connected"}},
		logs: []LogEntry{{
			ID: 1, Date: now.Format("2006/01/02"), Time: now.Format("15:04:05"), Timestamp: now.UnixMilli(),
			Level: "error", Node: "E2E Node", Container: "api", Message: "E2E error log", nodeID: "e2e-node",
		}},
		processed:             1,
		historyRange:          "30m",
		rules:                 map[string]bool{"mask": true, "structure": true, "noise": false},
		ruleOrder:             defaultRuleOrder(),
		subscribers:           make(map[chan LogEntry]struct{}),
		containerNames:        make(map[string]map[string]string),
		containerLogs:         make(map[string][]LogEntry),
		historyCoverage:       make(map[string]time.Time),
		historyLoads:          make(map[string]struct{}),
		historyLoadGeneration: make(map[string]uint64),
		streams:               make(map[string]struct{}),
		nodeContexts:          make(map[string]context.Context),
		nodeCancels:           make(map[string]context.CancelFunc),
		settings:              defaultAppSettings(),
	}
	web := httptest.NewServer(newHTTPHandler(s))
	t.Cleanup(web.Close)

	allocatorOptions := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	allocatorOptions = append(allocatorOptions,
		chromedp.ExecPath(browserPath),
		chromedp.Headless,
		chromedp.NoSandbox,
		chromedp.WindowSize(1440, 1000),
	)
	allocatorContext, cancelAllocator := chromedp.NewExecAllocator(context.Background(), allocatorOptions...)
	t.Cleanup(cancelAllocator)
	browserContext, cancelBrowser := chromedp.NewContext(allocatorContext,
		chromedp.WithErrorf(func(string, ...any) {}),
	)
	t.Cleanup(cancelBrowser)

	if err := chromedp.Run(browserContext,
		chromedp.Navigate(web.URL),
		chromedp.WaitVisible(`[data-analyze-log="1"]`, chromedp.ByQuery),
		chromedp.Click(`[data-analyze-log="1"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`#assistant-dock.open`, chromedp.ByQuery),
		chromedp.WaitVisible(`#assistant-context-list .assistant-context-item`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("open assistant from the first AI-button click: %v", err)
	}

	var contextCount, buttonPressed string
	if err := chromedp.Run(browserContext,
		chromedp.Text(`#assistant-context-count`, &contextCount, chromedp.ByQuery),
		chromedp.AttributeValue(`[data-analyze-log="1"]`, "aria-pressed", &buttonPressed, nil, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("read assistant selection state: %v", err)
	}
	if contextCount != "1" || buttonPressed != "true" {
		t.Fatalf("first click should select exactly one log, count=%q pressed=%q", contextCount, buttonPressed)
	}

	var persistedSessions, sessionMenuItems int
	if err := chromedp.Run(browserContext,
		chromedp.Click(`#assistant-session-toggle`, chromedp.ByQuery),
		chromedp.WaitVisible(`[data-select-assistant-session]`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelectorAll('[data-select-assistant-session]').length`, &sessionMenuItems),
		chromedp.Evaluate(`JSON.parse(localStorage.getItem('log-agent-ai-sessions') || '[]').length`, &persistedSessions),
		chromedp.Click(`#assistant-clear-context`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("manage analysis sessions: %v", err)
	}
	if sessionMenuItems != 1 || persistedSessions != 1 {
		t.Fatalf("the analyzed log should create one persisted session, menu=%d persisted=%d", sessionMenuItems, persistedSessions)
	}
	if err := chromedp.Run(browserContext,
		chromedp.Text(`#assistant-context-count`, &contextCount, chromedp.ByQuery),
		chromedp.Evaluate(`JSON.parse(localStorage.getItem('log-agent-ai-sessions') || '[]').length`, &persistedSessions),
	); err != nil {
		t.Fatalf("read new chat session state: %v", err)
	}
	if contextCount != "0" || persistedSessions != 2 {
		t.Fatalf("new chat should preserve history and clear its context, count=%q sessions=%d", contextCount, persistedSessions)
	}
	var sessionMenuOpen bool
	if err := chromedp.Run(browserContext,
		chromedp.Click(`#assistant-session-toggle`, chromedp.ByQuery),
		chromedp.Evaluate(`!document.querySelector('#assistant-session-menu').classList.contains('hidden')`, &sessionMenuOpen),
		chromedp.WaitVisible(`#assistant-session-list .assistant-session-item:nth-child(2) [data-select-assistant-session]`, chromedp.ByQuery),
		chromedp.Click(`#assistant-session-list .assistant-session-item:nth-child(2) [data-select-assistant-session]`, chromedp.ByQuery),
		chromedp.Text(`#assistant-context-count`, &contextCount, chromedp.ByQuery),
		// Selecting a session closes the menu, so reopen it before deleting.
		// The selected (now active) session is not necessarily :first-child:
		// syncActiveAssistantSession only promotes a session when a new one is
		// started, so a plain select leaves the list order untouched. Target
		// the active item explicitly to exercise "delete the active session".
		chromedp.Click(`#assistant-session-toggle`, chromedp.ByQuery),
		chromedp.WaitVisible(`#assistant-session-list .assistant-session-item.active [data-delete-assistant-session]`, chromedp.ByQuery),
		chromedp.Click(`#assistant-session-list .assistant-session-item.active [data-delete-assistant-session]`, chromedp.ByQuery),
		chromedp.Text(`#assistant-context-count`, &contextCount, chromedp.ByQuery),
		chromedp.Evaluate(`JSON.parse(localStorage.getItem('log-agent-ai-sessions') || '[]').length`, &persistedSessions),
	); err != nil {
		t.Fatalf("switch and delete analysis sessions: %v", err)
	}
	if !sessionMenuOpen || contextCount != "0" || persistedSessions != 1 {
		t.Fatalf("deleting the active history session should restore the remaining empty chat, menuOpen=%t count=%q sessions=%d", sessionMenuOpen, contextCount, persistedSessions)
	}

	attachmentPath := writeE2EPixel(t)
	if err := chromedp.Run(browserContext,
		chromedp.SetUploadFiles(`#assistant-file-input`, []string{attachmentPath}, chromedp.ByQuery),
		chromedp.WaitVisible(`[data-preview-assistant-attachment="0"]`, chromedp.ByQuery),
		chromedp.Click(`[data-preview-assistant-attachment="0"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`#assistant-attachment-preview-modal:not(.hidden)`, chromedp.ByQuery),
		chromedp.Click(`#assistant-attachment-preview-close`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("upload, preview, and close an attachment: %v", err)
	}

	var previewClosed, assistantOpen, wheelContained bool
	if err := chromedp.Run(browserContext,
		chromedp.Evaluate(`document.querySelector('#assistant-attachment-preview-modal').classList.contains('hidden')`, &previewClosed),
		chromedp.Evaluate(`document.querySelector('#assistant-dock').classList.contains('open')`, &assistantOpen),
		chromedp.Evaluate(`(() => {
			const heading = document.querySelector('#assistant-panel .assistant-heading');
			const event = new WheelEvent('wheel', { bubbles: true, cancelable: true, deltaY: 100 });
			heading.dispatchEvent(event);
			return event.defaultPrevented;
		})()`, &wheelContained),
	); err != nil {
		t.Fatalf("verify attachment preview close and scroll containment: %v", err)
	}
	if !previewClosed || !assistantOpen {
		t.Fatalf("closing preview must keep the assistant open, previewClosed=%t assistantOpen=%t", previewClosed, assistantOpen)
	}
	if !wheelContained {
		t.Fatal("wheel events over a non-scrollable assistant area must not reach the page")
	}

	var markdownSafety assistantMarkdownSafety
	if err := chromedp.Run(browserContext,
		chromedp.Evaluate(`(() => {
			window.__assistantMarkdownExecuted = false;
			const fence = String.fromCharCode(96).repeat(3);
			const inlineTick = String.fromCharCode(96);
			const markdown = '# 安全渲染\n#### 后 3 条日志\n> 引用内容\n---\n**系统任务调用 ' + inlineTick + 'ServiceException' + inlineTick + ' 失败。**\n[安全链接](https://example.com/docs)\n[危险链接](javascript:alert(1))\n<img src=x onerror="window.__assistantMarkdownExecuted=true">\n<script>window.__assistantMarkdownExecuted=true</script>\n' + fence + '\nconst ok = true;\n' + fence + '\n| 名称 | 结果 |\n| --- | --- |\n| 安全 | **通过** |';
			state.assistantMessages = [{ role: 'assistant', content: markdown }];
			renderAssistant();
			const message = document.querySelector('#assistant-messages .assistant-message.assistant');
			const safeLink = message.querySelector('a.assistant-markdown-link');
			return {
				hasCodeBlock: Boolean(message.querySelector('.markdown-code-block')),
				hasRawImage: Boolean(message.querySelector('img')),
				hasScriptTag: Boolean(message.querySelector('script')),
				hasTable: Boolean(message.querySelector('.markdown-table')),
				safeLink: safeLink?.getAttribute('href') === 'https://example.com/docs',
				unsafeLink: Array.from(message.querySelectorAll('a')).some((link) => /^javascript:/i.test(link.getAttribute('href') || '')),
				xssExecuted: window.__assistantMarkdownExecuted === true,
				sameBackground: getComputedStyle(message).backgroundColor === getComputedStyle(document.querySelector('#assistant-messages')).backgroundColor,
				hasDivider: Boolean(message.querySelector('.markdown-divider')),
				hasBlockquote: Boolean(message.querySelector('.markdown-blockquote')),
				hasDeepHeading: Array.from(message.querySelectorAll('h4')).some((heading) => heading.textContent.includes('后 3 条日志')),
				hasMixedBold: Array.from(message.querySelectorAll('strong')).some((strong) => strong.querySelector('code') && strong.textContent.includes('系统任务调用')),
			};
		})()`, &markdownSafety),
	); err != nil {
		t.Fatalf("verify markdown rendering safety: %v", err)
	}
	if !markdownSafety.HasCodeBlock || !markdownSafety.HasTable || !markdownSafety.SafeLink || !markdownSafety.SameBackground || !markdownSafety.HasDivider || !markdownSafety.HasBlockquote || !markdownSafety.HasDeepHeading || !markdownSafety.HasMixedBold || markdownSafety.HasRawImage || markdownSafety.HasScriptTag || markdownSafety.UnsafeLink || markdownSafety.XSSExecuted {
		t.Fatalf("unsafe markdown rendering state: %+v", markdownSafety)
	}

	var followUpMessagesSeparated bool
	if err := chromedp.Run(browserContext,
		chromedp.Evaluate(`(() => {
			const longAnswer = Array.from({ length: 18 }, (_, index) => (index + 1) + '. 这是用于验证长回答布局的内容，确保后续提问不会覆盖前一条回答。').join('\n');
			state.assistantMessages = [
				{ role: 'assistant', content: longAnswer },
				{ role: 'user', content: '这是继续提问' },
				{ role: 'assistant', content: '这是继续回答' },
			];
			renderAssistant();
			const items = Array.from(document.querySelectorAll('#assistant-messages .assistant-message'));
			return items.every((item, index) => index === 0 || item.getBoundingClientRect().top >= items[index - 1].getBoundingClientRect().bottom + 9);
		})()`, &followUpMessagesSeparated),
	); err != nil {
		t.Fatalf("verify assistant follow-up layout: %v", err)
	}
	if !followUpMessagesSeparated {
		t.Fatal("assistant follow-up messages must not overlap the previous long response")
	}
}

func firstExistingPath(paths ...string) string {
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func writeE2EPixel(t *testing.T) string {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLq7wAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatalf("decode test image: %v", err)
	}
	path := filepath.Join(t.TempDir(), "pixel.png")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write test image: %v", err)
	}
	return path
}

// storageModeView is what the settings panel looks like to a given caller.
type storageModeView struct {
	Mode            string `json:"mode"`
	Status          string `json:"status"`
	Note            string `json:"note"`
	SectionHidden   bool   `json:"sectionHidden"`
	PathsHidden     bool   `json:"pathsHidden"`
	ActionsHidden   bool   `json:"actionsHidden"`
	NodesPath       string `json:"nodesPath"`
	RevealDisabled  bool   `json:"revealDisabled"`
	LocalStorageKey string `json:"localStorageKey"`
}

// TestE2EStorageModeFollowsRequestOrigin drives the real page twice: once over
// the httptest loopback address (file mode) and once with a forged non-loopback
// RemoteAddr (browser mode). The rule the user asked for is fixed, not a user
// choice, so the page must reflect whatever /api/bootstrap reported.
func TestE2EStorageModeFollowsRequestOrigin(t *testing.T) {
	browserPath := firstExistingPath(
		`C:/Program Files\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:/Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	)
	if browserPath == "" {
		t.Skip("Chrome or Edge is required for UI end-to-end tests")
	}

	newServer := func(t *testing.T) *httptest.Server {
		t.Helper()
		// configDir() is memoized for the life of the process, so point it at a
		// scratch directory before the store resolves anything.
		t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
		*configDirCache() = configDirState{}
		t.Cleanup(func() { *configDirCache() = configDirState{} })
		s := &server{
			nodes:                 []Node{},
			logs:                  []LogEntry{},
			processed:             0,
			nextLogID:             0,
			historyRange:          "30m",
			rules:                 map[string]bool{"mask": true, "structure": true, "noise": false},
			ruleOrder:             defaultRuleOrder(),
			subscribers:           make(map[chan LogEntry]struct{}),
			containerNames:        make(map[string]map[string]string),
			containerLogs:         make(map[string][]LogEntry),
			historyCoverage:       make(map[string]time.Time),
			historyLoads:          make(map[string]struct{}),
			historyLoadGeneration: make(map[string]uint64),
			streams:               make(map[string]struct{}),
			nodeContexts:          make(map[string]context.Context),
			nodeCancels:           make(map[string]context.CancelFunc),
			settings:              defaultAppSettings(),
			store:                 newConfigStore(),
		}
		web := httptest.NewServer(newHTTPHandler(s))
		t.Cleanup(web.Close)
		return web
	}

	allocatorOptions := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	allocatorOptions = append(allocatorOptions,
		chromedp.ExecPath(browserPath),
		chromedp.Flag("headless", true),
		chromedp.NoSandbox,
	)

	inspect := func(t *testing.T, url string) storageModeView {
		t.Helper()
		allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocatorOptions...)
		defer cancelAlloc()
		ctx, cancel := chromedp.NewContext(allocCtx)
		defer cancel()

		var raw string
		if err := chromedp.Run(ctx,
			chromedp.Navigate(url),
			// The panel lives in a hidden modal; open it through the app's own
			// entry point so loadConfigInfo() and updateStorageModeUI() run, then
			// give the settings and config-info fetches time to settle.
			chromedp.Evaluate(`openAppSettings()`, nil),
			chromedp.Sleep(2*time.Second),
			chromedp.Evaluate(`JSON.stringify({
				status: document.querySelector('#settings-config-status')?.textContent || '',
				note: document.querySelector('#settings-config-note')?.textContent || '',
				sectionHidden: !!document.querySelector('#settings-config-storage')?.classList.contains('hidden'),
				pathsHidden: !!document.querySelector('#settings-config-paths')?.classList.contains('hidden'),
				actionsHidden: !!document.querySelector('#settings-config-actions')?.classList.contains('hidden'),
				nodesPath: document.querySelector('#settings-nodes-path')?.textContent || '',
				revealDisabled: !!(document.querySelector('#settings-config-reveal')||{}).disabled,
				localStorageKey: String(!!localStorage.getItem('log-agent-browser-nodes')),
			})`, &raw),
		); err != nil {
			t.Fatalf("inspect storage mode: %v", err)
		}
		var view storageModeView
		if err := json.Unmarshal([]byte(raw), &view); err != nil {
			t.Fatalf("decode panel view %q: %v", raw, err)
		}
		// The status text is the only mode signal a user sees.
		if view.Status == "本地 JSON 文件" {
			view.Mode = "file"
		} else if view.Status == "此浏览器" {
			view.Mode = "browser"
		}
		return view
	}

	t.Run("local caller sees file paths and actions", func(t *testing.T) {
		web := newServer(t)
		view := inspect(t, web.URL)
		if view.Mode != "file" {
			t.Fatalf("local caller mode = %q, want file", view.Mode)
		}
		if view.SectionHidden || view.PathsHidden || view.ActionsHidden {
			t.Fatalf("local caller must see the storage section: %+v", view)
		}
		if view.NodesPath == "" || view.NodesPath == "—" {
			t.Fatalf("local caller must see the resolved node path, got %q", view.NodesPath)
		}
		if view.Status != "本地 JSON 文件" {
			t.Fatalf("local caller status = %q, want 本地 JSON 文件", view.Status)
		}
	})

	t.Run("remote caller is routed to browser storage", func(t *testing.T) {
		// httptest always connects from 127.0.0.1, so rewrite RemoteAddr to a
		// public address to act as a visitor on someone else's deployment.
		s := &server{
			nodes:                 []Node{},
			logs:                  []LogEntry{},
			historyRange:          "30m",
			rules:                 map[string]bool{"mask": true, "structure": true, "noise": false},
			ruleOrder:             defaultRuleOrder(),
			subscribers:           make(map[chan LogEntry]struct{}),
			containerNames:        make(map[string]map[string]string),
			containerLogs:         make(map[string][]LogEntry),
			historyCoverage:       make(map[string]time.Time),
			historyLoads:          make(map[string]struct{}),
			historyLoadGeneration: make(map[string]uint64),
			streams:               make(map[string]struct{}),
			nodeContexts:          make(map[string]context.Context),
			nodeCancels:           make(map[string]context.CancelFunc),
			settings:              defaultAppSettings(),
			store:                 newConfigStore(),
		}
		handler := newHTTPHandler(s)
		web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.RemoteAddr = "124.174.71.198:51234"
			handler.ServeHTTP(w, r)
		}))
		t.Cleanup(web.Close)

		view := inspect(t, web.URL)
		if view.Mode != "browser" {
			t.Fatalf("remote caller mode = %q, want browser", view.Mode)
		}
		// The whole point of browser mode: no filesystem affordances at all.
		if !view.SectionHidden {
			t.Fatal("remote caller must not see the file-backed storage section")
		}
		if view.NodesPath != "" && view.NodesPath != "—" {
			t.Fatalf("remote caller leaked a node path: %q", view.NodesPath)
		}
		if view.Status != "此浏览器" {
			t.Fatalf("remote caller status = %q, want 此浏览器", view.Status)
		}
		if !strings.Contains(view.Note, "浏览器") {
			t.Fatalf("remote caller note must explain browser storage, got %q", view.Note)
		}
	})
}
