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
//
// The reveal link is the whole storage-mode signal in the UI: there is no
// status text and no path row any more, so RevealHidden is what Mode derives
// from.
type storageModeView struct {
	Mode            string `json:"mode"`
	Note            string `json:"note"`
	SectionHidden   bool   `json:"sectionHidden"`
	ActionsHidden   bool   `json:"actionsHidden"`
	RevealPresent   bool   `json:"revealPresent"`
	RevealHidden    bool   `json:"revealHidden"`
	HeaderHasReveal bool   `json:"headerHasReveal"`
	LocalStorageKey string `json:"localStorageKey"`
}

// TestE2EStorageModeFollowsRequestOrigin drives the real page twice: once over
// the httptest loopback address (file mode) and once with a forged non-loopback
// RemoteAddr (browser mode). The rule the user asked for is fixed, not a user
// choice, so the page must reflect whatever /api/bootstrap reported.
//
// The single intended difference between the two panels is the reveal link in
// the modal header, so that link's visibility is what each subtest asserts on.
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
				note: document.querySelector('#settings-config-note')?.textContent || '',
				sectionHidden: !!document.querySelector('#settings-config-storage')?.classList.contains('hidden'),
				actionsHidden: !!document.querySelector('#settings-config-actions')?.classList.contains('hidden'),
				revealPresent: !!document.querySelector('#settings-config-reveal'),
				revealHidden: !!document.querySelector('#settings-config-reveal')?.classList.contains('hidden'),
				headerHasReveal: !!document.querySelector('#app-settings-modal .modal-header-actions #settings-config-reveal'),
				localStorageKey: String(!!localStorage.getItem('log-agent-browser-nodes')),
			})`, &raw),
		); err != nil {
			t.Fatalf("inspect storage mode: %v", err)
		}
		var view storageModeView
		if err := json.Unmarshal([]byte(raw), &view); err != nil {
			t.Fatalf("decode panel view %q: %v", raw, err)
		}
		if !view.RevealPresent {
			t.Fatalf("the reveal link must exist in the markup for both modes")
		}
		// The link's visibility is the only mode signal a user sees.
		if view.RevealHidden {
			view.Mode = "browser"
		} else {
			view.Mode = "file"
		}
		return view
	}

	t.Run("local caller sees the reveal link", func(t *testing.T) {
		web := newServer(t)
		view := inspect(t, web.URL)
		if view.Mode != "file" {
			t.Fatalf("local caller mode = %q, want file", view.Mode)
		}
		if view.RevealHidden {
			t.Fatalf("local caller must see the file-manager link: %+v", view)
		}
		// The header is where the link was asked to live, so pin the place down
		// in case a future layout change moves it back into the section body.
		if !view.HeaderHasReveal {
			t.Fatalf("the reveal link must sit in the modal header: %+v", view)
		}
		if view.SectionHidden || view.ActionsHidden {
			t.Fatalf("export and import must stay available locally: %+v", view)
		}
		if !strings.Contains(view.Note, "data") {
			t.Fatalf("local caller note must describe file storage, got %q", view.Note)
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
		// The whole point of browser mode: no filesystem affordance at all. The
		// link is the only one, so hiding it is the complete requirement.
		if !view.RevealHidden {
			t.Fatal("remote caller must not see the file-manager link")
		}
		if !strings.Contains(view.Note, "浏览器") {
			t.Fatalf("remote caller note must explain browser storage, got %q", view.Note)
		}
		// Export and import are storage-agnostic (they read and write whatever
		// backend is active), so they stay visible in both modes.
		if view.SectionHidden || view.ActionsHidden {
			t.Fatalf("export and import must stay available remotely: %+v", view)
		}
	})
}

// aiProfileStorageView is what the model-settings panel shows after a load.
type aiProfileStorageView struct {
	ProfileNames    []string `json:"profileNames"`
	LocalStorageLen int      `json:"localStorageLen"`
	StorageMode     string   `json:"storageMode"`
}

// TestE2EAIProfilesComeFromFileNotBrowserCache pins down the storage source of
// the model settings panel.
//
// The bug this covers: loadAIProfilesFromFile() used to merge the providers it
// fetched into whatever localStorage already held, and returned early when the
// file was empty. A browser that had stale providers cached therefore kept
// showing them on a file-backed page, and the merge wrote the file's providers
// back into localStorage, so a machine's plaintext API keys travelled to every
// later remote visit. The file has to be the authority in file mode.
func TestE2EAIProfilesComeFromFileNotBrowserCache(t *testing.T) {
	browserPath := firstExistingPath(
		`C:/Program Files\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:/Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	)
	if browserPath == "" {
		t.Skip("Chrome or Edge is required for UI end-to-end tests")
	}

	// A file-backed store holding exactly one provider.
	staleName := "来自浏览器缓存的残留供应商"
	configRoot := t.TempDir()
	t.Setenv("LOG_AGENT_CONFIG_DIR", configRoot)
	resetConfigDirCache(t)
	t.Cleanup(func() { *configDirCache() = configDirState{} })

	store := newConfigStore()
	if _, err := store.upsertModel(0, "文件里的供应商", "https://api.example.com/v1", "sk-from-file", 0, []string{"file-model"}); err != nil {
		t.Fatalf("seed models.json: %v", err)
	}

	s := &server{
		nodes:                 []Node{},
		logs:                  []LogEntry{},
		historyRange:          "30m",
		rules:                 map[string]bool{},
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
		store:                 store,
	}
	web := httptest.NewServer(newHTTPHandler(s))
	t.Cleanup(web.Close)

	allocatorOptions := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	allocatorOptions = append(allocatorOptions,
		chromedp.ExecPath(browserPath),
		chromedp.Flag("headless", true),
		chromedp.NoSandbox,
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocatorOptions...)
	t.Cleanup(cancelAlloc)
	ctx, cancel := chromedp.NewContext(allocCtx)
	t.Cleanup(cancel)

	// Seed localStorage with a provider that only ever existed in the browser,
	// then load the page: it must not appear, because this page is file-backed.
	if err := chromedp.Run(ctx,
		chromedp.Navigate(web.URL),
		chromedp.Evaluate(`localStorage.setItem('log-agent-ai-profiles', JSON.stringify([{
			id: 'ai-profile-stale-1',
			name: '`+staleName+`',
			baseURL: 'https://stale.example.com/v1',
			apiKey: 'sk-stale-should-not-surface',
			type: 'openai',
			enabled: true,
			models: [{ id: 'stale-model-1', name: 'stale-model' }],
		}]))`, nil),
	); err != nil {
		t.Fatalf("seed browser cache: %v", err)
	}

	// Reload so bootstrap runs with the stale cache already in place.
	var view aiProfileStorageView
	var raw string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(web.URL),
		chromedp.Sleep(2*time.Second),
		chromedp.Evaluate(`JSON.stringify({
			profileNames: state.aiProfiles.map((profile) => profile.name),
			localStorageLen: JSON.parse(localStorage.getItem('log-agent-ai-profiles') || '[]').length,
			storageMode: state.storageMode,
		})`, &raw),
	); err != nil {
		t.Fatalf("inspect model settings: %v", err)
	}
	if err := json.Unmarshal([]byte(raw), &view); err != nil {
		t.Fatalf("decode model settings view %q: %v", raw, err)
	}

	if view.StorageMode != "file" {
		t.Fatalf("loopback page should be file-backed, got mode %q", view.StorageMode)
	}
	for _, name := range view.ProfileNames {
		if name == staleName {
			t.Fatalf("file-backed page surfaced a cached provider: %+v", view.ProfileNames)
		}
	}
	if len(view.ProfileNames) != 1 || view.ProfileNames[0] != "文件里的供应商" {
		t.Fatalf("file-backed page must show exactly models.json contents, got %+v", view.ProfileNames)
	}
	// The file's provider must not be copied into the browser cache either.
	// Otherwise a stale copy survives here, and the next visit that does render
	// from localStorage (a remote visitor, or this machine after the file is
	// emptied) shows providers that no longer exist on disk.
	//
	// Note this is a leak of configuration, not of credentials: aiProfileView
	// deliberately omits APIKey, so the key never reaches the browser at all.
	// That is why the assertion is on the cache contents rather than on a secret.
	if view.LocalStorageLen != 1 {
		t.Fatalf("file mode must not rewrite the browser cache, len=%d", view.LocalStorageLen)
	}
	var cacheRaw string
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`localStorage.getItem('log-agent-ai-profiles') || '[]'`, &cacheRaw),
	); err != nil {
		t.Fatalf("read browser cache: %v", err)
	}
	if strings.Contains(cacheRaw, "文件里的供应商") {
		t.Fatalf("file-mode provider leaked into the browser cache: %s", cacheRaw)
	}
	if !strings.Contains(cacheRaw, staleName) {
		t.Fatalf("the browser cache should still hold only its own provider, got %s", cacheRaw)
	}
}

// TestE2EBrowserModeKeepsItsOwnProviders is the other half of the storage split:
// a remote visitor must still see the providers it configured itself. The fix
// that stops file mode from reading the browser cache must not turn into
// "ignore localStorage everywhere".
func TestE2EBrowserModeKeepsItsOwnProviders(t *testing.T) {
	browserPath := firstExistingPath(
		`C:/Program Files\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:/Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	)
	if browserPath == "" {
		t.Skip("Chrome or Edge is required for UI end-to-end tests")
	}

	// The store is empty, so anything rendered here can only come from the
	// browser's own cache.
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	resetConfigDirCache(t)
	t.Cleanup(func() { *configDirCache() = configDirState{} })

	s := &server{
		nodes:                 []Node{},
		logs:                  []LogEntry{},
		historyRange:          "30m",
		rules:                 map[string]bool{},
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

	allocatorOptions := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	allocatorOptions = append(allocatorOptions,
		chromedp.ExecPath(browserPath),
		chromedp.Flag("headless", true),
		chromedp.NoSandbox,
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocatorOptions...)
	t.Cleanup(cancelAlloc)
	ctx, cancel := chromedp.NewContext(allocCtx)
	t.Cleanup(cancel)

	ownName := "访客自己的供应商"
	if err := chromedp.Run(ctx,
		chromedp.Navigate(web.URL),
		chromedp.Evaluate(`localStorage.setItem('log-agent-ai-profiles', JSON.stringify([{
			id: 'visitor-1',
			name: '`+ownName+`',
			baseURL: 'https://visitor.example.com/v1',
			apiKey: 'sk-visitor',
			type: 'openai',
			enabled: true,
			models: [{ id: 'visitor-model-1', name: 'visitor-model' }],
		}]))`, nil),
	); err != nil {
		t.Fatalf("seed visitor cache: %v", err)
	}

	var view aiProfileStorageView
	var raw string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(web.URL),
		chromedp.Sleep(2*time.Second),
		chromedp.Evaluate(`JSON.stringify({
			profileNames: state.aiProfiles.map((profile) => profile.name),
			localStorageLen: JSON.parse(localStorage.getItem('log-agent-ai-profiles') || '[]').length,
			storageMode: state.storageMode,
		})`, &raw),
	); err != nil {
		t.Fatalf("inspect model settings: %v", err)
	}
	if err := json.Unmarshal([]byte(raw), &view); err != nil {
		t.Fatalf("decode model settings view %q: %v", raw, err)
	}
	if view.StorageMode != "browser" {
		t.Fatalf("remote page should be browser-backed, got mode %q", view.StorageMode)
	}
	if len(view.ProfileNames) != 1 || view.ProfileNames[0] != ownName {
		t.Fatalf("remote page must show its own cached providers, got %+v", view.ProfileNames)
	}
}

// sidebarLayoutView is the measured geometry of the sidebar's stacked children.
type sidebarLayoutView struct {
	ListTop       float64 `json:"listTop"`
	ListBottom    float64 `json:"listBottom"`
	ButtonTop     float64 `json:"buttonTop"`
	ButtonBottom  float64 `json:"buttonBottom"`
	FooterTop     float64 `json:"footerTop"`
	FooterBottom  float64 `json:"footerBottom"`
	SidebarBottom float64 `json:"sidebarBottom"`
	ViewportH     float64 `json:"viewportH"`
}

// TestE2ESidebarNodeListFillsAvailableHeight pins the sidebar layout.
//
// The node list used to be capped at min(62vh, 560px) while the footer claimed
// the remaining slack with margin-top: auto. With only a couple of nodes the
// list stopped growing early, so the add-node button sat just below it and a
// tall dead gap opened between the button and the status footer. The list now
// stretches to fill that space, which places the button at the bottom.
//
// Geometry is asserted rather than class names because the bug was purely a
// matter of where things ended up; a rule-level assertion would have passed
// both before and after.
func TestE2ESidebarNodeListFillsAvailableHeight(t *testing.T) {
	browserPath := firstExistingPath(
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	)
	if browserPath == "" {
		t.Skip("Chrome or Edge is required for UI end-to-end tests")
	}

	// Two nodes: few enough that the old cap left the sidebar visibly unfilled.
	now := time.Now()
	s := &server{
		nodes: []Node{
			{ID: "node-a", Name: "溯帆", URL: "http://124.174.71.198:8099", Status: "connected", Initial: "溯"},
			{ID: "node-b", Name: "掌门人", URL: "https://115.190.152.177:8999", Status: "connected", Initial: "掌"},
		},
		logs:                  []LogEntry{{ID: 1, Date: now.Format("2006/01/02"), Time: now.Format("15:04:05"), Timestamp: now.UnixMilli(), Level: "info", Node: "溯帆", Container: "api", Message: "hello", nodeID: "node-a"}},
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
		// A desktop-sized viewport; the sidebar collapses below 680px, where
		// the button and footer are hidden by design.
		chromedp.WindowSize(1440, 1000),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocatorOptions...)
	t.Cleanup(cancelAlloc)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithErrorf(func(string, ...any) {}))
	t.Cleanup(cancel)

	if err := chromedp.Run(ctx,
		chromedp.Navigate(web.URL),
		chromedp.WaitVisible(`#open-add-node`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("load dashboard: %v", err)
	}

	var raw string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		const box = (selector) => document.querySelector(selector).getBoundingClientRect();
		const list = box('#node-list');
		const button = box('#open-add-node');
		const footer = box('.sidebar-footer');
		return JSON.stringify({
			listTop: list.top, listBottom: list.bottom,
			buttonTop: button.top, buttonBottom: button.bottom,
			footerTop: footer.top, footerBottom: footer.bottom,
			sidebarBottom: box('.sidebar').bottom,
			viewportH: window.innerHeight,
		});
	})()`, &raw)); err != nil {
		t.Fatalf("measure sidebar: %v", err)
	}
	var layout sidebarLayoutView
	if err := json.Unmarshal([]byte(raw), &layout); err != nil {
		t.Fatalf("decode sidebar layout %q: %v", raw, err)
	}

	// The list must reach the button. The residual gap is the button's own
	// 15px top margin, so the tolerance covers that plus rounding.
	if gap := layout.ButtonTop - layout.ListBottom; gap > 20 {
		t.Fatalf("node list stops %.0fpx short of the add-node button; want it to stretch (layout %+v)", gap, layout)
	}
	// And the button must sit directly above the footer, so the empty space
	// lives inside the scrollable list instead of beneath the button.
	if gap := layout.FooterTop - layout.ButtonBottom; gap > 40 {
		t.Fatalf("add-node button floats %.0fpx above the footer; want it pushed to the bottom (layout %+v)", gap, layout)
	}
	// The sidebar should still span the viewport with no trailing gap.
	if slack := layout.ViewportH - layout.SidebarBottom; slack > 4 {
		t.Fatalf("sidebar is %.0fpx shorter than the viewport (layout %+v)", slack, layout)
	}
}

// nodeMultiSelectView is read back from the page as a JSON string. It records
// the highlighted node ids plus the switch's visual state, because the bug this
// guards against (a switch that flips but does not actually gate the click) is
// invisible if only the state variable is asserted.
type nodeMultiSelectView struct {
	Active      []string `json:"active"`
	ToggleOn    bool     `json:"toggleOn"`
	AriaChecked string   `json:"ariaChecked"`
}

func readNodeMultiSelectView(t *testing.T, ctx context.Context) nodeMultiSelectView {
	t.Helper()
	var raw string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		const toggle = document.querySelector('#node-multi-select-toggle');
		return JSON.stringify({
			active: Array.from(document.querySelectorAll('#node-list .node-item.active')).map((item) => item.dataset.nodeId),
			toggleOn: toggle.classList.contains('active'),
			ariaChecked: toggle.getAttribute('aria-checked'),
		});
	})()`, &raw)); err != nil {
		t.Fatalf("read multi-select state: %v", err)
	}
	var view nodeMultiSelectView
	if err := json.Unmarshal([]byte(raw), &view); err != nil {
		t.Fatalf("decode multi-select state %q: %v", raw, err)
	}
	return view
}

func clickNode(t *testing.T, ctx context.Context, nodeID string) {
	t.Helper()
	if err := chromedp.Run(ctx, chromedp.Click(`#node-list .node-item[data-node-id="`+nodeID+`"]`, chromedp.ByQuery)); err != nil {
		t.Fatalf("click node %s: %v", nodeID, err)
	}
}

func clickMultiSelectToggle(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := chromedp.Run(ctx, chromedp.Click(`#node-multi-select-toggle`, chromedp.ByQuery)); err != nil {
		t.Fatalf("click multi-select toggle: %v", err)
	}
}

// TestE2ENodeMultiSelectToggle covers the sidebar switch that gates node
// multi-selection. The default must stay single-select, otherwise the switch
// would be a no-op; turning it off again must actually collapse a multi-node
// selection instead of leaving several nodes highlighted.
func TestE2ENodeMultiSelectToggle(t *testing.T) {
	browserPath := firstExistingPath(
		`C:/Program Files\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:/Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	)
	if browserPath == "" {
		t.Skip("Chrome or Edge is required for UI end-to-end tests")
	}
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	*configDirCache() = configDirState{}

	now := time.Now()
	s := &server{
		nodes: []Node{
			{ID: "node-a", Name: "溯帆", URL: "http://124.174.71.198:8099", Status: "connected", Initial: "溯"},
			{ID: "node-b", Name: "掌门人", URL: "https://115.190.152.177:8999", Status: "connected", Initial: "掌"},
			{ID: "node-c", Name: "美圃", URL: "http://124.174.71.40:8099", Status: "connected", Initial: "美"},
		},
		logs:                  []LogEntry{{ID: 1, Date: now.Format("2006/01/02"), Time: now.Format("15:04:05"), Timestamp: now.UnixMilli(), Level: "info", Node: "溯帆", Container: "api", Message: "hello", nodeID: "node-a"}},
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
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocatorOptions...)
	t.Cleanup(cancelAlloc)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithErrorf(func(string, ...any) {}))
	t.Cleanup(cancel)

	if err := chromedp.Run(ctx,
		chromedp.Navigate(web.URL),
		chromedp.WaitVisible(`#node-list .node-item[data-node-id="node-a"]`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("load dashboard: %v", err)
	}

	// Default: switch off, and clicks replace the selection.
	initial := readNodeMultiSelectView(t, ctx)
	if initial.ToggleOn || initial.AriaChecked != "false" {
		t.Fatalf("multi-select switch should default to off, got %+v", initial)
	}

	clickNode(t, ctx, "node-a")
	clickNode(t, ctx, "node-b")
	single := readNodeMultiSelectView(t, ctx)
	if len(single.Active) != 1 || single.Active[0] != "node-b" {
		t.Fatalf("with multi-select off the second click should replace the selection, got %+v", single)
	}

	// Turn the switch on: clicks become additive and a click on a selected node
	// removes it again.
	clickMultiSelectToggle(t, ctx)
	afterToggle := readNodeMultiSelectView(t, ctx)
	if !afterToggle.ToggleOn || afterToggle.AriaChecked != "true" {
		t.Fatalf("multi-select switch should be on after clicking it, got %+v", afterToggle)
	}

	clickNode(t, ctx, "node-a")
	clickNode(t, ctx, "node-c")
	multi := readNodeMultiSelectView(t, ctx)
	if len(multi.Active) != 3 {
		t.Fatalf("multi-select on should accumulate nodes, got %+v", multi)
	}

	clickNode(t, ctx, "node-c")
	deselected := readNodeMultiSelectView(t, ctx)
	if len(deselected.Active) != 2 {
		t.Fatalf("clicking a selected node should deselect it, got %+v", deselected)
	}

	// Turning the switch back off has to collapse the selection to one node.
	clickMultiSelectToggle(t, ctx)
	collapsed := readNodeMultiSelectView(t, ctx)
	if collapsed.ToggleOn || collapsed.AriaChecked != "false" {
		t.Fatalf("multi-select switch should be off after the second click, got %+v", collapsed)
	}
	if len(collapsed.Active) != 1 {
		t.Fatalf("turning multi-select off should narrow the selection to one node, got %+v", collapsed)
	}
}

// TestE2EMultiSelectRestoreIsConsistent guards a state that the switch itself
// cannot currently produce but that saved selections can: several nodes
// highlighted while the switch reads "off". Any payload written before the
// switch existed carries `nodes: [a, b]` and no `multiSelect`, and a reload
// renders exactly that contradiction.
func TestE2EMultiSelectRestoreIsConsistent(t *testing.T) {
	browserPath := firstExistingPath(
		`C:/Program Files\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:/Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	)
	if browserPath == "" {
		t.Skip("Chrome or Edge is required for UI end-to-end tests")
	}
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	*configDirCache() = configDirState{}

	now := time.Now()
	s := &server{
		nodes: []Node{
			{ID: "node-a", Name: "溯帆", URL: "http://124.174.71.198:8099", Status: "connected", Initial: "溯"},
			{ID: "node-b", Name: "掌门人", URL: "https://115.190.152.177:8999", Status: "connected", Initial: "掌"},
		},
		logs:                  []LogEntry{{ID: 1, Date: now.Format("2006/01/02"), Time: now.Format("15:04:05"), Timestamp: now.UnixMilli(), Level: "info", Node: "溯帆", Container: "api", Message: "hello", nodeID: "node-a"}},
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
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocatorOptions...)
	t.Cleanup(cancelAlloc)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithErrorf(func(string, ...any) {}))
	t.Cleanup(cancel)

	// Seed a legacy payload, then reload so restoreSelection() consumes it.
	if err := chromedp.Run(ctx,
		chromedp.Navigate(web.URL),
		chromedp.WaitVisible(`#open-add-node`, chromedp.ByQuery),
		chromedp.Evaluate(`localStorage.setItem('log-agent-selection', JSON.stringify({nodes:['node-a','node-b'],containers:[]}))`, nil),
		chromedp.Reload(),
		chromedp.WaitVisible(`#node-list .node-item`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("seed legacy selection: %v", err)
	}

	view := readNodeMultiSelectView(t, ctx)
	if len(view.Active) > 1 && !view.ToggleOn {
		t.Fatalf("switch reads off but %d nodes stay highlighted: the control contradicts the selection (%+v)", len(view.Active), view)
	}
}

// commandsColumnView captures the measured geometry of the three command
// columns. Layout bugs here are about where boxes land, so the assertions use
// pixel positions rather than class names, which pass either way.
type commandsColumnView struct {
	LeftLeft     float64 `json:"leftLeft"`
	LeftRight    float64 `json:"leftRight"`
	LeftTop      float64 `json:"leftTop"`
	LeftBottom   float64 `json:"leftBottom"`
	MidLeft      float64 `json:"midLeft"`
	MidRight     float64 `json:"midRight"`
	MidTop       float64 `json:"midTop"`
	MidBottom    float64 `json:"midBottom"`
	RightLeft    float64 `json:"rightLeft"`
	RightRight   float64 `json:"rightRight"`
	RightTop     float64 `json:"rightTop"`
	RightBottom  float64 `json:"rightBottom"`
	ServerTop    float64 `json:"serverTop"`
	ShelfTop     float64 `json:"shelfTop"`
	ServerBottom float64 `json:"serverBottom"`
	ShelfBottom  float64 `json:"shelfBottom"`
	IntroBottom  float64 `json:"introBottom"`
	IntroLeft    float64 `json:"introLeft"`
	IntroRight   float64 `json:"introRight"`
	PageLeft     float64 `json:"pageLeft"`
	PageRight    float64 `json:"pageRight"`
	BodyScrollW  float64 `json:"bodyScrollW"`
	ViewportW    float64 `json:"viewportW"`
}

// TestE2ECommandsColumnsLayout asserts the server-commands page lays out as three
// equal-height columns: servers over the file shelf at ~25%, the flow list at
// ~35%, and the editor at ~40%, all starting at the page's existing left margin.
func TestE2ECommandsColumnsLayout(t *testing.T) {
	browserPath := firstExistingPath(
		`C:/Program Files\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:/Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	)
	if browserPath == "" {
		t.Skip("Chrome or Edge is required for UI end-to-end tests")
	}
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	*configDirCache() = configDirState{}

	s := &server{
		nodes:                 []Node{{ID: "node-a", Name: "溯帆", URL: "http://124.174.71.198:8099", Status: "connected", Initial: "溯"}},
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
		// Wide enough that the 1180px breakpoint does not kick in.
		chromedp.WindowSize(1600, 1000),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocatorOptions...)
	t.Cleanup(cancelAlloc)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithErrorf(func(string, ...any) {}))
	t.Cleanup(cancel)

	if err := chromedp.Run(ctx,
		chromedp.Navigate(web.URL),
		chromedp.WaitVisible(`#open-add-node`, chromedp.ByQuery),
		chromedp.Evaluate(`setView('commands')`, nil),
		chromedp.WaitVisible(`.commands-columns`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("open commands view: %v", err)
	}

	var raw string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		const box = (sel) => { const el = document.querySelector(sel); return el ? el.getBoundingClientRect() : null; };
		const left = box('.commands-column-left');
		const mid = box('.commands-column-center');
		const right = box('.commands-column-right');
		const server = box('.server-library');
		const shelf = box('#command-file-shelf');
		const intro = box('#commands-view .page-intro');
		const page = box('#commands-view');
		return JSON.stringify({
			leftLeft: left.left, leftRight: left.right, leftTop: left.top, leftBottom: left.bottom,
			midLeft: mid.left, midRight: mid.right, midTop: mid.top, midBottom: mid.bottom,
			rightLeft: right.left, rightRight: right.right, rightTop: right.top, rightBottom: right.bottom,
			serverTop: server.top, serverBottom: server.bottom, shelfTop: shelf.top, shelfBottom: shelf.bottom,
			introBottom: intro.bottom, introLeft: intro.left, introRight: intro.right,
			pageLeft: page.left, pageRight: page.right,
			bodyScrollW: document.documentElement.scrollWidth, viewportW: window.innerWidth,
		});
	})()`, &raw)); err != nil {
		t.Fatalf("measure commands columns: %v", err)
	}
	var v commandsColumnView
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("decode commands layout %q: %v", raw, err)
	}

	// The three columns sit side by side, in order, without overlapping.
	if !(v.LeftRight <= v.MidLeft+0.5) {
		t.Fatalf("left column overlaps the middle one: left.right=%.1f mid.left=%.1f (%+v)", v.LeftRight, v.MidLeft, v)
	}
	if !(v.MidRight <= v.RightLeft+0.5) {
		t.Fatalf("middle column overlaps the right one: mid.right=%.1f right.left=%.1f (%+v)", v.MidRight, v.RightLeft, v)
	}

	// Widths should track 25/35/40 of the grid width. Compare against the sum of
	// the three columns' outer span so page padding is excluded.
	leftW, midW, rightW := v.LeftRight-v.LeftLeft, v.MidRight-v.MidLeft, v.RightRight-v.RightLeft
	span := v.RightRight - v.LeftLeft
	if span <= 0 {
		t.Fatalf("columns span is not positive: %+v", v)
	}
	for _, c := range []struct {
		name string
		got  float64
		want float64
	}{{"left", leftW, 25}, {"middle", midW, 35}, {"right", rightW, 40}} {
		share := c.got / span * 100
		if diff := share - c.want; diff > 2.5 || diff < -2.5 {
			t.Fatalf("%s column is %.1f%% of the row, want ~%.0f%% (measured %+v)", c.name, share, c.want, v)
		}
	}

	// Equal height: the three columns must share a top and a bottom edge.
	if d := v.MidTop - v.LeftTop; d > 1 || d < -1 {
		t.Fatalf("middle column top %.1f differs from left %.1f (%+v)", v.MidTop, v.LeftTop, v)
	}
	if d := v.RightTop - v.LeftTop; d > 1 || d < -1 {
		t.Fatalf("right column top %.1f differs from left %.1f (%+v)", v.RightTop, v.LeftTop, v)
	}
	if d := v.MidBottom - v.LeftBottom; d > 1 || d < -1 {
		t.Fatalf("middle column bottom %.1f differs from left %.1f (%+v)", v.MidBottom, v.LeftBottom, v)
	}
	if d := v.RightBottom - v.LeftBottom; d > 1 || d < -1 {
		t.Fatalf("right column bottom %.1f differs from left %.1f (%+v)", v.RightBottom, v.LeftBottom, v)
	}

	// The first column stacks servers above the file shelf, with the grid gap.
	if !(v.ServerBottom <= v.ShelfTop) {
		t.Fatalf("server library (bottom %.1f) should sit above the file shelf (top %.1f) (%+v)", v.ServerBottom, v.ShelfTop, v)
	}
	if gap := v.ShelfTop - v.ServerBottom; gap < 8 || gap > 40 {
		t.Fatalf("stacked panels are %.1fpx apart, want the grid gap (~16px) (%+v)", gap, v)
	}

	// The columns begin below the page intro and keep the page's existing left
	// margin. The reference is the intro's own left edge, not #commands-view's:
	// the view IS .page-content and carries the 39px horizontal padding, so its
	// border box sits 39px left of where its content actually starts.
	if !(v.LeftTop >= v.IntroBottom-1) {
		t.Fatalf("columns (top %.1f) should start below the intro (bottom %.1f) (%+v)", v.LeftTop, v.IntroBottom, v)
	}
	if d := v.LeftLeft - v.IntroLeft; d > 1 || d < -1 {
		t.Fatalf("columns left edge %.1f does not line up with the page intro left edge %.1f (%+v)", v.LeftLeft, v.IntroLeft, v)
	}
	if d := v.IntroRight - v.RightRight; d > 1 || d < -1 {
		t.Fatalf("columns right edge %.1f does not line up with the page intro right edge %.1f (%+v)", v.RightRight, v.IntroRight, v)
	}

	// No horizontal overflow from the new grid.
	if v.BodyScrollW > v.ViewportW+1 {
		t.Fatalf("page overflows horizontally: scrollWidth=%.1f viewport=%.1f (%+v)", v.BodyScrollW, v.ViewportW, v)
	}
}

// commandsScrollView measures whether the command workspace confines overflow to
// its own columns. The failure it guards against is the page growing a scrollbar
// once servers, files and command lines accumulate, so the assertions compare the
// document against the viewport and confirm each list really does overflow
// (otherwise the fixture would prove nothing).
type commandsScrollView struct {
	DocScrollH         float64 `json:"docScrollH"`
	DocClientH         float64 `json:"docClientH"`
	ColumnsTop         float64 `json:"columnsTop"`
	ColumnsH           float64 `json:"columnsH"`
	ServerListH        float64 `json:"serverListH"`
	ServerListScrollH  float64 `json:"serverListScrollH"`
	ServerListOverflow string  `json:"serverListOverflow"`
	FlowListH          float64 `json:"flowListH"`
	FlowListScrollH    float64 `json:"flowListScrollH"`
	FlowListOverflow   string  `json:"flowListOverflow"`
	EditorBodyH        float64 `json:"editorBodyH"`
	EditorBodyOverflow string  `json:"editorBodyOverflow"`
	ViewportH          float64 `json:"viewportH"`
}

// TestE2ECommandsPanelsScrollInsteadOfStretching seeds enough servers and flows
// through localStorage to overflow a short viewport, then asserts the overflow
// is confined to the column lists instead of stretching the page.
//
// Command servers and flows live only in the browser (there is no server-side
// model for them), so the fixture has to go through localStorage rather than the
// Go server struct.
func TestE2ECommandsPanelsScrollInsteadOfStretching(t *testing.T) {
	browserPath := firstExistingPath(
		`C:/Program Files\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:/Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	)
	if browserPath == "" {
		t.Skip("Chrome or Edge is required for UI end-to-end tests")
	}
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	*configDirCache() = configDirState{}

	s := &server{
		nodes:                 []Node{{ID: "node-a", Name: "溯帆", URL: "http://124.174.71.198:8099", Status: "connected", Initial: "溯"}},
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
		// A short viewport makes the overflow pressure obvious.
		chromedp.WindowSize(1600, 820),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocatorOptions...)
	t.Cleanup(cancelAlloc)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithErrorf(func(string, ...any) {}))
	t.Cleanup(cancel)

	// 12 servers and 10 flows: comfortably more than one short screenful.
	seed := `(() => {
		const servers = [];
		for (let i = 0; i < 12; i++) {
			servers.push({ id: 'srv-' + i, name: '服务器 ' + (i + 1), host: '192.168.1.' + (i + 20), port: '22', user: 'deploy', auth: 'key', secret: '' });
		}
		const flows = [];
		for (let i = 0; i < 10; i++) {
			flows.push({ id: 'flow-' + i, name: '测试 ' + (i + 1), lines: [{ id: 'l-' + i, text: 'ls -la' }], bindings: [] });
		}
		localStorage.setItem('log-agent-command-servers', JSON.stringify(servers));
		localStorage.setItem('log-agent-command-flows', JSON.stringify(flows));
		return servers.length + '/' + flows.length;
	})()`

	if err := chromedp.Run(ctx,
		chromedp.Navigate(web.URL),
		chromedp.WaitVisible(`#open-add-node`, chromedp.ByQuery),
		chromedp.Evaluate(seed, nil),
		chromedp.Reload(),
		chromedp.WaitVisible(`#open-add-node`, chromedp.ByQuery),
		chromedp.Evaluate(`setView('commands')`, nil),
		chromedp.WaitVisible(`.commands-columns`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("seed and open commands view: %v", err)
	}

	var raw string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		const el = (sel) => document.querySelector(sel);
		const of = (sel) => { const n = el(sel); if (!n) return 'missing'; return getComputedStyle(n).overflowY; };
		const cols = el('.commands-columns').getBoundingClientRect();
		const serverList = el('#server-card-list');
		const flowList = el('#commands-flow-list');
		const editorBody = el('.command-editor-body');
		return JSON.stringify({
			docScrollH: document.documentElement.scrollHeight,
			docClientH: document.documentElement.clientHeight,
			columnsTop: cols.top, columnsH: cols.height,
			serverListH: serverList ? serverList.getBoundingClientRect().height : -1,
			serverListScrollH: serverList ? serverList.scrollHeight : -1,
			serverListOverflow: of('#server-card-list'),
			flowListH: flowList ? flowList.getBoundingClientRect().height : -1,
			flowListScrollH: flowList ? flowList.scrollHeight : -1,
			flowListOverflow: of('#commands-flow-list'),
			editorBodyH: editorBody ? editorBody.getBoundingClientRect().height : -1,
			editorBodyOverflow: of('.command-editor-body'),
			viewportH: window.innerHeight,
		});
	})()`, &raw)); err != nil {
		t.Fatalf("measure commands scroll: %v", err)
	}
	var v commandsScrollView
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("decode commands scroll %q: %v", raw, err)
	}

	// The whole point: no page-level scrollbar from the panel content.
	if v.DocScrollH > v.DocClientH+2 {
		t.Fatalf("page grew a scrollbar from panel content: scrollHeight=%.1f clientHeight=%.1f (%+v)", v.DocScrollH, v.DocClientH, v)
	}
	if v.ColumnsTop+v.ColumnsH > v.ViewportH+2 {
		t.Fatalf("columns row overflows the viewport: bottom=%.1f viewport=%.1f (%+v)", v.ColumnsTop+v.ColumnsH, v.ViewportH, v)
	}
	// The scrolling must be real, otherwise the fixture proves nothing.
	if v.ServerListOverflow != "auto" {
		t.Fatalf("server list overflow-y should be auto to confine scrolling, got %q (%+v)", v.ServerListOverflow, v)
	}
	if v.ServerListScrollH <= v.ServerListH+1 {
		t.Fatalf("server list did not overflow: scrollHeight=%.1f height=%.1f — increase the fixture (%+v)", v.ServerListScrollH, v.ServerListH, v)
	}
	if v.FlowListOverflow != "auto" {
		t.Fatalf("flow list overflow-y should be auto, got %q (%+v)", v.FlowListOverflow, v)
	}
	if v.FlowListScrollH <= v.FlowListH+1 {
		t.Fatalf("flow list did not overflow: scrollHeight=%.1f height=%.1f (%+v)", v.FlowListScrollH, v.FlowListH, v)
	}
	if v.EditorBodyOverflow != "auto" {
		t.Fatalf("editor body overflow-y should be auto, got %q (%+v)", v.EditorBodyOverflow, v)
	}
}

// fileShelfFitsView reports whether the file shelf's contents stay inside the
// shelf's own box, and whether the server library still scrolls.
type fileShelfFitsView struct {
	ShelfTop           float64 `json:"shelfTop"`
	ShelfBottom        float64 `json:"shelfBottom"`
	ShelfH             float64 `json:"shelfH"`
	ShelfScrollH       float64 `json:"shelfScrollH"`
	ShelfOverflow      string  `json:"shelfOverflow"`
	ShelfMinHeight     string  `json:"shelfMinHeight"`
	DropZoneTop        float64 `json:"dropZoneTop"`
	DropZoneBottom     float64 `json:"dropZoneBottom"`
	DropZoneVisible    bool    `json:"dropZoneVisible"`
	FileListBottom     float64 `json:"fileListBottom"`
	ServerLibH         float64 `json:"serverLibH"`
	ServerListH        float64 `json:"serverListH"`
	ServerListSh       float64 `json:"serverListSh"`
	ServerListOverflow string  `json:"serverListOverflow"`
	DocScrollH         float64 `json:"docScrollH"`
	DocClientH         float64 `json:"docClientH"`
}

// TestE2EFileShelfIsNotClippedByTheLeftColumn guards the bug where the file
// shelf was shrunk below its own content. The left column splits its height
// between the server library and the shelf; when the shelf was `flex:0 1 auto`
// the library's demand squashed it, and `overflow:hidden` clipped the drop zone
// completely off the panel -- the user saw a "上传文件" heading with an empty
// body. The shelf must therefore keep a content floor, and the server library
// (whose list genuinely scrolls) is what gives up the space.
func TestE2EFileShelfIsNotClippedByTheLeftColumn(t *testing.T) {
	browserPath := firstExistingPath(
		`C:/Program Files\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:/Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	)
	if browserPath == "" {
		t.Skip("Chrome or Edge is required for UI end-to-end tests")
	}
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	*configDirCache() = configDirState{}

	s := &server{
		nodes:                 []Node{{ID: "node-a", Name: "溯帆", URL: "http://124.174.71.198:8099", Status: "connected", Initial: "溯"}},
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
		// The reporter's viewport was short; the squeeze only shows there.
		chromedp.WindowSize(1440, 900),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocatorOptions...)
	t.Cleanup(cancelAlloc)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithErrorf(func(string, ...any) {}))
	t.Cleanup(cancel)

	// Three servers is what the report showed; the squeeze came from the
	// library's intrinsic demand, not from an extreme fixture.
	seed := `(() => {
		const servers = [];
		for (let i = 0; i < 3; i++) {
			servers.push({ id: 'srv-' + i, name: '服务器 ' + (i + 1), host: '192.168.1.' + (i + 20), port: '22', user: 'deploy', auth: 'key', secret: '' });
		}
		const flows = [{ id: 'flow-0', name: '测试', lines: [{ id: 'l-0', text: '' }], bindings: [] }];
		localStorage.setItem('log-agent-command-servers', JSON.stringify(servers));
		localStorage.setItem('log-agent-command-flows', JSON.stringify(flows));
		return 'ok';
	})()`

	var raw string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(web.URL),
		chromedp.WaitVisible(`#open-add-node`, chromedp.ByQuery),
		chromedp.Evaluate(seed, nil),
		chromedp.Reload(),
		chromedp.WaitVisible(`#open-add-node`, chromedp.ByQuery),
		chromedp.Evaluate(`setView('commands')`, nil),
		chromedp.WaitVisible(`.commands-columns`, chromedp.ByQuery),
		chromedp.Sleep(300*time.Millisecond),
		chromedp.Evaluate(`(() => {
			const el = (sel) => document.querySelector(sel);
			const shelf = el('#command-file-shelf');
			const shelfBox = shelf.getBoundingClientRect();
			const zone = el('#command-file-drop-zone');
			const zoneBox = zone.getBoundingClientRect();
			const list = el('#command-file-list');
			const listBox = list.getBoundingClientRect();
			const serverList = el('#server-card-list');
			return JSON.stringify({
				shelfTop: shelfBox.top, shelfBottom: shelfBox.bottom, shelfH: shelfBox.height,
				shelfScrollH: shelf.scrollHeight,
				shelfOverflow: getComputedStyle(shelf).overflowY,
				shelfMinHeight: getComputedStyle(shelf).minHeight,
				dropZoneTop: zoneBox.top, dropZoneBottom: zoneBox.bottom,
				// The zone counts as visible only if its whole box sits inside the
				// shelf's clip rect; a partially clipped zone is the bug.
				dropZoneVisible: zoneBox.top >= shelfBox.top - 0.5 && zoneBox.bottom <= shelfBox.bottom + 0.5,
				fileListBottom: listBox.bottom,
				serverLibH: el('.server-library').getBoundingClientRect().height,
				serverListH: serverList.getBoundingClientRect().height,
				serverListSh: serverList.scrollHeight,
				serverListOverflow: getComputedStyle(serverList).overflowY,
				docScrollH: document.documentElement.scrollHeight,
				docClientH: document.documentElement.clientHeight,
			});
		})()`, &raw)); err != nil {
		t.Fatalf("measure file shelf: %v", err)
	}
	var v fileShelfFitsView
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("decode file shelf %q: %v", raw, err)
	}

	// The reported symptom: the drop zone fell outside the shelf and was clipped.
	if !v.DropZoneVisible {
		t.Fatalf("drop zone is clipped by the shelf: zone=%.1f..%.1f shelf=%.1f..%.1f (%+v)",
			v.DropZoneTop, v.DropZoneBottom, v.ShelfTop, v.ShelfBottom, v)
	}
	if v.FileListBottom > v.ShelfBottom+0.5 {
		t.Fatalf("file list bottom %.1f spills past the shelf bottom %.1f (%+v)", v.FileListBottom, v.ShelfBottom, v)
	}

	// The shelf must not be squashed below its own content.
	if v.ShelfScrollH > v.ShelfH+1 {
		t.Fatalf("shelf content (%.1f) exceeds its height (%.1f) — it was squeezed below its chrome (%+v)", v.ShelfScrollH, v.ShelfH, v)
	}

	// The server library is the one that absorbs the pressure by scrolling.
	if v.ServerListOverflow != "auto" {
		t.Fatalf("server list overflow-y should be auto, got %q (%+v)", v.ServerListOverflow, v)
	}
	if v.ServerListSh <= v.ServerListH+1 {
		t.Fatalf("server list did not overflow: scrollHeight=%.1f height=%.1f — increase the fixture (%+v)", v.ServerListSh, v.ServerListH, v)
	}

	// Fixing the shelf must not reintroduce a page-level scrollbar.
	if v.DocScrollH > v.DocClientH+2 {
		t.Fatalf("page grew a scrollbar: scrollHeight=%.1f clientHeight=%.1f (%+v)", v.DocScrollH, v.DocClientH, v)
	}
}

// narrowColumnView measures whether each commands panel keeps its own content
// inside its box when the responsive 2-column layout kicks in.
type narrowColumnView struct {
	ViewportW float64 `json:"viewportW"`
	// The shelf's own box and the elements that must sit inside it.
	ShelfTop       float64 `json:"shelfTop"`
	ShelfBottom    float64 `json:"shelfBottom"`
	ShelfH         float64 `json:"shelfH"`
	DropZoneTop    float64 `json:"dropZoneTop"`
	DropZoneBottom float64 `json:"dropZoneBottom"`
	DropZoneH      float64 `json:"dropZoneH"`
	FileListBottom float64 `json:"fileListBottom"`
	// The board heading must not wrap its title to a second line.
	FlowTitleH       float64 `json:"flowTitleH"`
	FlowActionsWidth float64 `json:"flowActionsWidth"`
	AddFlowLabelRows float64 `json:"addFlowLabelRows"`
}

// TestE2ENarrowLayoutKeepsPanelsWhole covers the reported "上传文件 还是没显示全"
// at a narrow window, where `.commands-columns` collapses to 2 columns.
//
// Two things went wrong there. The left column became a 2-column grid with
// `align-items:start`, so the server library grew to its natural height and
// pushed the file shelf out of the column; and the shelf was left with no
// content floor, so the column could squeeze it below its own heading and
// `overflow:hidden` clipped the drop zone away. Separately the board heading
// squeezed its action buttons until the labels wrapped.
func TestE2ENarrowLayoutKeepsPanelsWhole(t *testing.T) {
	browserPath := firstExistingPath(
		`C:/Program Files\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:/Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	)
	if browserPath == "" {
		t.Skip("Chrome or Edge is required for UI end-to-end tests")
	}
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	*configDirCache() = configDirState{}

	s := &server{
		nodes:                 []Node{{ID: "node-a", Name: "溯帆", Status: "connected"}},
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
		// Below the 1180px breakpoint, so the left column is a 2-column grid.
		chromedp.WindowSize(1092, 600),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocatorOptions...)
	t.Cleanup(cancelAlloc)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithErrorf(func(string, ...any) {}))
	t.Cleanup(cancel)

	seed := `(() => {
		const servers = [
			{ id: 'srv-0', name: 'apac', host: '192.168.1.10', port: '22', user: 'x', auth: 'key', secret: '', connectionStatus: 'untested' },
			{ id: 'srv-1', name: '溯帆服务器', host: '124.174.71.198', port: '22', user: 'root', auth: 'password', secret: 'x', connectionStatus: 'success' },
			{ id: 'srv-2', name: '服务器 3', host: '192.168.1.20', port: '22', user: 'deploy', auth: 'key', secret: '', connectionStatus: 'untested' },
		];
		const flows = [{ id: 'flow-0', name: '测试', lines: [{ id: 'l-0', text: '' }], bindings: [{ serverId: 'srv-1' }] }];
		localStorage.setItem('log-agent-command-servers', JSON.stringify(servers));
		localStorage.setItem('log-agent-command-flows', JSON.stringify(flows));
		return 'ok';
	})()`

	var raw string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(web.URL),
		chromedp.WaitVisible(`#open-add-node`, chromedp.ByQuery),
		chromedp.Evaluate(seed, nil),
		chromedp.Reload(),
		chromedp.WaitVisible(`#open-add-node`, chromedp.ByQuery),
		chromedp.Evaluate(`setView('commands')`, nil),
		chromedp.WaitVisible(`.commands-columns`, chromedp.ByQuery),
		chromedp.Sleep(400*time.Millisecond),
		chromedp.Evaluate(`(() => {
			const el = (sel) => document.querySelector(sel);
			const box = (sel) => el(sel).getBoundingClientRect();
			const shelfBox = box('#command-file-shelf');
			const zoneBox = box('#command-file-drop-zone');
			const listBox = box('#command-file-list');
			const titleEl = el('#commands-view .commands-board h2');
			// How many text lines the add-flow button label occupies: compare its
			// rendered height against a single line. A wrapped label is the bug.
			const addFlow = el('#commands-add-flow');
			const addFlowBox = addFlow.getBoundingClientRect();
			return JSON.stringify({
				viewportW: window.innerWidth,
				shelfTop: shelfBox.top, shelfBottom: shelfBox.bottom, shelfH: shelfBox.height,
				dropZoneTop: zoneBox.top, dropZoneBottom: zoneBox.bottom, dropZoneH: zoneBox.height,
				fileListBottom: listBox.bottom,
				flowTitleH: titleEl ? titleEl.getBoundingClientRect().height : -1,
				flowActionsWidth: box('.command-board-actions').width,
				// A single-line 30px button stays 30px; wrapped text makes it taller.
				addFlowLabelRows: addFlowBox.height,
			});
		})()`, &raw)); err != nil {
		t.Fatalf("measure narrow layout: %v", err)
	}
	var v narrowColumnView
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("decode narrow layout %q: %v", raw, err)
	}

	// The whole drop zone must sit inside the shelf, not hang below it.
	if v.DropZoneTop < v.ShelfTop-0.5 || v.DropZoneBottom > v.ShelfBottom+0.5 {
		t.Fatalf("drop zone is clipped at the narrow layout: zone=%.1f..%.1f shelf=%.1f..%.1f (%+v)",
			v.DropZoneTop, v.DropZoneBottom, v.ShelfTop, v.ShelfBottom, v)
	}
	// The shelf must be tall enough for its own chrome, and the drop zone must
	// keep its designed height rather than being squeezed by the column.
	if v.ShelfH < 190 {
		t.Fatalf("file shelf is only %.1fpx tall -- too short for its heading plus drop zone (%+v)", v.ShelfH, v)
	}
	if v.DropZoneH < 80 {
		t.Fatalf("drop zone squeezed to %.1fpx, want ~84px (%+v)", v.DropZoneH, v)
	}
	if v.FileListBottom > v.ShelfBottom+0.5 {
		t.Fatalf("file list bottom %.1f spills past the shelf bottom %.1f (%+v)", v.FileListBottom, v.ShelfBottom, v)
	}

	// The board title must stay on one line (a wrapped title was a visible
	// regression once the actions stopped shrinking).
	if v.FlowTitleH > 34 {
		t.Fatalf("指令集组 title wrapped to %.1fpx tall -- the heading should stack instead (%+v)", v.FlowTitleH, v)
	}
	// Buttons keep a single line; a squeezed label would make the button taller.
	if v.AddFlowLabelRows > 36 {
		t.Fatalf("新增指令集 button grew to %.1fpx -- its label wrapped (%+v)", v.AddFlowLabelRows, v)
	}
}
