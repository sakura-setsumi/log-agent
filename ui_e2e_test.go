package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
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
	// The label is read from the page's own language table: the UI ships English
	// by default and switches to Chinese, so hardcoding either wording here would
	// break the moment the copy changes.
	if pausedView.Processed != "1" || !pausedView.Paused || pausedView.NewLogCount != 0 || pausedView.StreamStatus != i18nLabel(t, browserContext, "stream.status.paused") {
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
	if pausedStatus != i18nLabel(t, browserContext, "stream.status.paused") {
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

// i18nLabel reads a copy string out of the page's own language table. The UI ships
// English and can switch to Chinese, so a test that hardcoded either wording would
// break the moment the copy changes -- asking the page keeps the assertion about
// behaviour instead.
func i18nLabel(t *testing.T, ctx context.Context, key string) string {
	t.Helper()
	var label string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`t('`+key+`')`, &label)); err != nil {
		t.Fatalf("read i18n label %q: %v", key, err)
	}
	return label
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
	BrowserNote     string `json:"browserNote"`
	FileNote        string `json:"fileNote"`
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
				browserNote: t('settings.configNoteBrowser'),
				fileNote: t('settings.configNotePlain'),
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
		// The copy follows the UI language (English by default), so compare it
		// against the page's own dictionary instead of hard-coding a language.
		if view.Note != view.BrowserNote || view.Note == view.FileNote {
			t.Fatalf("remote caller note must be the browser-storage wording: %+v", view)
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
	FooterTop     float64 `json:"footerTop"`
	FooterBottom  float64 `json:"footerBottom"`
	SidebarBottom float64 `json:"sidebarBottom"`
	ViewportH     float64 `json:"viewportH"`
}

// TestE2ESidebarNodeListFillsAvailableHeight pins the sidebar layout.
//
// The node list used to be capped at min(62vh, 560px) while the footer claimed
// the remaining slack with margin-top: auto. With only a couple of nodes the
// list stopped growing early and a tall dead gap opened between it and the
// status footer. The list now stretches to fill that space.
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
		// the footer is hidden by design.
		chromedp.WindowSize(1440, 1000),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocatorOptions...)
	t.Cleanup(cancelAlloc)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithErrorf(func(string, ...any) {}))
	t.Cleanup(cancel)

	if err := chromedp.Run(ctx,
		chromedp.Navigate(web.URL),
		chromedp.WaitVisible(`#node-list`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("load dashboard: %v", err)
	}

	var raw string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		const box = (selector) => document.querySelector(selector).getBoundingClientRect();
		const list = box('#node-list');
		const footer = box('.sidebar-footer');
		return JSON.stringify({
			listTop: list.top, listBottom: list.bottom,
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

	// The list must reach the footer, so the empty space lives inside the
	// scrollable list. The residual gap is the footer's own 15px top margin,
	// so the tolerance covers that plus rounding.
	if gap := layout.FooterTop - layout.ListBottom; gap > 20 {
		t.Fatalf("node list stops %.0fpx short of the footer; want it to stretch (layout %+v)", gap, layout)
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

	// Seed the stale payload the buggy build leaves behind: two nodes AND a
	// container scope from each, with no multiSelect flag at all. Then reload so
	// restoreSelection() consumes it.
	if err := chromedp.Run(ctx,
		chromedp.Navigate(web.URL),
		chromedp.WaitVisible(`#node-list`, chromedp.ByQuery),
		chromedp.Evaluate(`localStorage.setItem('log-agent-selection', JSON.stringify({nodes:['node-a','node-b'],containers:['node-a::api','node-b::web']}))`, nil),
		chromedp.Reload(),
		chromedp.WaitVisible(`#node-list .node-item`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("seed legacy selection: %v", err)
	}

	view := readNodeMultiSelectView(t, ctx)
	if len(view.Active) > 1 && !view.ToggleOn {
		t.Fatalf("switch reads off but %d nodes stay highlighted: the control contradicts the selection (%+v)", len(view.Active), view)
	}

	// Narrowing the nodes is not enough on its own: a container scope belonging
	// to a node that is no longer selected would keep the stream silently
	// filtered down to nothing. Only the surviving node's scopes may remain.
	var stray []string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		const active = new Set(Array.from(document.querySelectorAll('#node-list .node-item.active')).map((item) => item.dataset.nodeId));
		return state.selectedContainers.filter((key) => !active.has(key.split('::')[0]));
	})()`, &stray)); err != nil {
		t.Fatalf("read restored container scopes: %v", err)
	}
	if len(stray) != 0 {
		t.Fatalf("container scopes survived for nodes that are not selected: %v (active=%v)", stray, view.Active)
	}
}

// commandsColumnView captures the measured geometry of the three command
// columns. Layout bugs here are about where boxes land, so the assertions use
// pixel positions rather than class names, which pass either way.
type commandsColumnView struct {
	MidLeft     float64 `json:"midLeft"`
	MidRight    float64 `json:"midRight"`
	MidTop      float64 `json:"midTop"`
	MidBottom   float64 `json:"midBottom"`
	RightLeft   float64 `json:"rightLeft"`
	RightRight  float64 `json:"rightRight"`
	RightTop    float64 `json:"rightTop"`
	RightBottom float64 `json:"rightBottom"`
	IntroBottom float64 `json:"introBottom"`
	IntroLeft   float64 `json:"introLeft"`
	IntroRight  float64 `json:"introRight"`
	// The dock: two buttons at the bottom-right.
	FilesLeft       float64 `json:"filesLeft"`
	FilesTop        float64 `json:"filesTop"`
	ServersRight    float64 `json:"serversRight"`
	ServersLeft     float64 `json:"serversLeft"`
	ServersTop      float64 `json:"serversTop"`
	AIRight         float64 `json:"aiRight"`
	AITop           float64 `json:"aiTop"`
	MainLeft        float64 `json:"mainLeft"`
	MainRight       float64 `json:"mainRight"`
	AddServerInMenu bool    `json:"addServerInMenu"`
	NodePanelShown  bool    `json:"nodePanelShown"`
	LibraryHidden   bool    `json:"libraryHidden"`
	ShelfHidden     bool    `json:"shelfHidden"`
	BodyScrollW     float64 `json:"bodyScrollW"`
	ViewportW       float64 `json:"viewportW"`
}

// TestE2ECommandsColumnsLayout asserts the server-commands page lays out as two
// equal-height columns -- the flow list at ~44% and the editor at ~56% -- with
// the server library and the file shelf out of the page: "new server" sits in the
// application menu and the two panels are popups behind a pair of buttons at the
// bottom-right, aligned with the page gutter.
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
		chromedp.WaitVisible(`#node-list`, chromedp.ByQuery),
		chromedp.Evaluate(`setView('commands')`, nil),
		chromedp.WaitVisible(`.commands-columns`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("open commands view: %v", err)
	}

	var raw string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		const box = (sel) => { const el = document.querySelector(sel); return el ? el.getBoundingClientRect() : null; };
		const mid = box('.commands-column-center');
		const right = box('.commands-column-right');
		const intro = box('#commands-view .page-intro');
		const files = box('#command-dock-files');
		const servers = box('#command-dock-servers');
		const ai = box('#assistant-launcher');
		const main = box('.main-content');
		const addServer = document.querySelector('#commands-add-server');
		const hidden = (sel) => document.querySelector(sel).getBoundingClientRect().height === 0;
		return JSON.stringify({
			midLeft: mid.left, midRight: mid.right, midTop: mid.top, midBottom: mid.bottom,
			rightLeft: right.left, rightRight: right.right, rightTop: right.top, rightBottom: right.bottom,
			introBottom: intro.bottom, introLeft: intro.left, introRight: intro.right,
			filesLeft: files.left, filesTop: files.top,
			serversLeft: servers.left, serversRight: servers.right, serversTop: servers.top,
			aiRight: ai.right, aiTop: ai.top,
			mainLeft: main.left, mainRight: main.right,
			addServerInMenu: !!addServer && !!addServer.closest('#menubar .menu-panel'),
			nodePanelShown: getComputedStyle(document.querySelector('.sidebar')).display !== 'none',
			libraryHidden: hidden('.server-library'), shelfHidden: hidden('#command-file-shelf'),
			bodyScrollW: document.documentElement.scrollWidth, viewportW: window.innerWidth,
		});
	})()`, &raw)); err != nil {
		t.Fatalf("measure commands columns: %v", err)
	}
	var v commandsColumnView
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("decode commands layout %q: %v", raw, err)
	}

	// The two columns sit side by side, in order, without overlapping.
	if !(v.MidRight <= v.RightLeft+0.5) {
		t.Fatalf("middle column overlaps the right one: mid.right=%.1f right.left=%.1f (%+v)", v.MidRight, v.RightLeft, v)
	}

	// Widths should track 44/56 of the grid width.
	midW, rightW := v.MidRight-v.MidLeft, v.RightRight-v.RightLeft
	span := v.RightRight - v.MidLeft
	if span <= 0 {
		t.Fatalf("columns span is not positive: %+v", v)
	}
	for _, c := range []struct {
		name string
		got  float64
		want float64
	}{{"middle", midW, 44}, {"right", rightW, 56}} {
		share := c.got / span * 100
		if diff := share - c.want; diff > 3 || diff < -3 {
			t.Fatalf("%s column is %.1f%% of the row, want ~%.0f%% (measured %+v)", c.name, share, c.want, v)
		}
	}

	// Equal height: the columns must share a top and a bottom edge.
	if d := v.RightTop - v.MidTop; d > 1 || d < -1 {
		t.Fatalf("right column top %.1f differs from middle %.1f (%+v)", v.RightTop, v.MidTop, v)
	}
	if d := v.RightBottom - v.MidBottom; d > 1 || d < -1 {
		t.Fatalf("right column bottom %.1f differs from middle %.1f (%+v)", v.RightBottom, v.MidBottom, v)
	}

	// The columns begin below the page intro and keep the page's existing left
	// margin. The reference is the intro's own left edge, not #commands-view's:
	// the view is .page-content and carries the shared horizontal gutter, so
	// its border box starts before its padded content.
	if !(v.MidTop >= v.IntroBottom-1) {
		t.Fatalf("columns (top %.1f) should start below the intro (bottom %.1f) (%+v)", v.MidTop, v.IntroBottom, v)
	}
	if d := v.MidLeft - v.IntroLeft; d > 1 || d < -1 {
		t.Fatalf("columns left edge %.1f does not line up with the page intro left edge %.1f (%+v)", v.MidLeft, v.IntroLeft, v)
	}
	if d := v.IntroRight - v.RightRight; d > 1 || d < -1 {
		t.Fatalf("columns right edge %.1f does not line up with the page intro right edge %.1f (%+v)", v.RightRight, v.IntroRight, v)
	}

	// The Dozzle node panel is not shown on this page; only the icon rail is.
	if v.NodePanelShown {
		t.Fatalf("the node panel should be hidden on the server-commands page (%+v)", v)
	}

	// "New server" lives in the application menu; the library and the shelf are
	// closed popups until their buttons are used.
	if !v.AddServerInMenu {
		t.Fatalf("新增服务器 should be an item in the application menu (%+v)", v)
	}
	if !v.LibraryHidden || !v.ShelfHidden {
		t.Fatalf("server library / file shelf should start closed (%+v)", v)
	}

	// The command dock is aligned to the right page gutter.
	if v.AIRight != 0 || v.AITop != 0 {
		t.Fatalf("AI launcher should be hidden on the commands page (%+v)", v)
	}
	if inset := v.MainRight - v.ServersRight; inset < 23 || inset > 25 {
		t.Fatalf("command dock right inset is %.1fpx, want 24px (%+v)", inset, v)
	}
	if !(v.FilesLeft < v.ServersLeft) || v.ServersTop != v.FilesTop {
		t.Fatalf("dock buttons should sit side by side, files first (%+v)", v)
	}
	if v.MidBottom > v.FilesTop+0.5 {
		t.Fatalf("columns (bottom %.1f) run under the dock buttons (top %.1f) (%+v)", v.MidBottom, v.FilesTop, v)
	}

	// No horizontal overflow from the grid.
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
		// Flows load from data/commands.json, which wins over localStorage.
		const request = new XMLHttpRequest();
		request.open('PUT', '/api/commands/state', false);
		request.setRequestHeader('Content-Type', 'application/json');
		request.send(JSON.stringify({ flows, servers }));
		if (request.status !== 200) throw new Error('seed commands.json: ' + request.status);
		return servers.length + '/' + flows.length;
	})()`

	if err := chromedp.Run(ctx,
		chromedp.Navigate(web.URL),
		chromedp.WaitVisible(`#node-list`, chromedp.ByQuery),
		chromedp.Evaluate(seed, nil),
		chromedp.Reload(),
		chromedp.WaitVisible(`#node-list`, chromedp.ByQuery),
		chromedp.Evaluate(`setView('commands')`, nil),
		chromedp.WaitVisible(`.commands-columns`, chromedp.ByQuery),
		// The server list lives in the library popup.
		chromedp.Click(`#command-dock-servers`, chromedp.ByQuery),
		chromedp.WaitVisible(`#server-card-list`, chromedp.ByQuery),
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
	PopupTop           float64 `json:"popupTop"`
	PopupBottom        float64 `json:"popupBottom"`
	DockTop            float64 `json:"dockTop"`
	LibraryOpen        bool    `json:"libraryOpen"`
	ServerListOverflow string  `json:"serverListOverflow"`
	DocScrollH         float64 `json:"docScrollH"`
	DocClientH         float64 `json:"docClientH"`
}

// TestE2EFileShelfPopupIsNotClipped guards the file shelf against being shrunk
// below its own content. The shelf is a popup above the dock, and
// `overflow:hidden` clips whatever does not fit, so a squeezed shelf shows a
// "上传文件" heading with an empty body. The shelf must keep a content floor and
// stay inside the viewport above its button; opening it closes the library
// popup, whose list genuinely scrolls.
func TestE2EFileShelfPopupIsNotClipped(t *testing.T) {
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
		// Flows load from data/commands.json, which wins over localStorage.
		const request = new XMLHttpRequest();
		request.open('PUT', '/api/commands/state', false);
		request.setRequestHeader('Content-Type', 'application/json');
		request.send(JSON.stringify({ flows, servers }));
		if (request.status !== 200) throw new Error('seed commands.json: ' + request.status);
		return 'ok';
	})()`

	var raw string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(web.URL),
		chromedp.WaitVisible(`#node-list`, chromedp.ByQuery),
		chromedp.Evaluate(seed, nil),
		chromedp.Reload(),
		chromedp.WaitVisible(`#node-list`, chromedp.ByQuery),
		chromedp.Evaluate(`setView('commands')`, nil),
		chromedp.WaitVisible(`.commands-columns`, chromedp.ByQuery),
		chromedp.Click(`#command-dock-files`, chromedp.ByQuery),
		chromedp.WaitVisible(`#command-file-drop-zone`, chromedp.ByQuery),
		chromedp.Sleep(400*time.Millisecond),
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
				popupTop: el('#command-popup-files').getBoundingClientRect().top,
				popupBottom: el('#command-popup-files').getBoundingClientRect().bottom,
				dockTop: el('#command-dock-files').getBoundingClientRect().top,
				libraryOpen: el('.server-library').getBoundingClientRect().height > 0,
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

	// The popup sits fully on screen, directly above its button, and opening the
	// shelf closes the library.
	if v.PopupTop < 0 || v.PopupBottom > v.DockTop+0.5 {
		t.Fatalf("file popup is not above the dock inside the viewport: popup=%.1f..%.1f dock.top=%.1f (%+v)", v.PopupTop, v.PopupBottom, v.DockTop, v)
	}
	if v.LibraryOpen {
		t.Fatalf("server library should be closed while the file shelf is open (%+v)", v)
	}
	// The library's list absorbs pressure by scrolling inside its popup.
	if v.ServerListOverflow != "auto" {
		t.Fatalf("server list overflow-y should be auto, got %q (%+v)", v.ServerListOverflow, v)
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
// The shelf (now a popup) must keep a content floor so `overflow:hidden` cannot
// clip the drop zone away, and the board heading must not squeeze its action
// buttons until the labels wrap.
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
		// Below the 1180px breakpoint, so the columns collapse.
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
		// Flows load from data/commands.json, which wins over localStorage.
		const request = new XMLHttpRequest();
		request.open('PUT', '/api/commands/state', false);
		request.setRequestHeader('Content-Type', 'application/json');
		request.send(JSON.stringify({ flows, servers }));
		if (request.status !== 200) throw new Error('seed commands.json: ' + request.status);
		return 'ok';
	})()`

	var raw string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(web.URL),
		chromedp.WaitVisible(`#node-list`, chromedp.ByQuery),
		chromedp.Evaluate(seed, nil),
		chromedp.Reload(),
		chromedp.WaitVisible(`#node-list`, chromedp.ByQuery),
		chromedp.Evaluate(`setView('commands')`, nil),
		chromedp.WaitVisible(`.commands-columns`, chromedp.ByQuery),
		chromedp.Click(`#command-dock-files`, chromedp.ByQuery),
		chromedp.WaitVisible(`#command-file-drop-zone`, chromedp.ByQuery),
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

// containerScopeView captures what the user actually sees after container
// clicks: which nodes are highlighted, the switch position, the detail panel's
// node caption, and the active container scopes. The reported bug is a
// contradiction between the highlight/caption and the switch, so all of them
// are read from the DOM rather than from state.
type containerScopeView struct {
	Active     []string `json:"active"`
	ToggleOn   bool     `json:"toggleOn"`
	Caption    string   `json:"caption"`
	Containers []string `json:"containers"`
}

func readContainerScopeView(t *testing.T, ctx context.Context) containerScopeView {
	t.Helper()
	var raw string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		const toggle = document.querySelector('#node-multi-select-toggle');
		const caption = document.querySelector('#stream-caption');
		return JSON.stringify({
			active: Array.from(document.querySelectorAll('#node-list .node-item.active')).map((item) => item.dataset.nodeId),
			toggleOn: toggle.classList.contains('active'),
			caption: caption ? caption.textContent : '',
			containers: Array.from(document.querySelectorAll('#node-list .container-item.active')).map((item) => item.dataset.nodeId + '::' + item.dataset.containerId),
		});
	})()`, &raw)); err != nil {
		t.Fatalf("read container scope: %v", err)
	}
	var view containerScopeView
	if err := json.Unmarshal([]byte(raw), &view); err != nil {
		t.Fatalf("decode container scope %q: %v", raw, err)
	}
	return view
}

// ensureNodeExpanded expands a node's container list if it is not already open.
// It must be idempotent: the expand button toggles, so clicking it twice
// collapses the list and any later container click would wait forever for an
// element that no longer exists.
func ensureNodeExpanded(t *testing.T, ctx context.Context, nodeID string) {
	t.Helper()
	var present bool
	query := `document.querySelector('#node-list .node-children[data-container-list-for="` + nodeID + `"]') !== null`
	if err := chromedp.Run(ctx, chromedp.Evaluate(query, &present)); err != nil {
		t.Fatalf("read expansion of node %s: %v", nodeID, err)
	}
	if present {
		return
	}
	if err := chromedp.Run(ctx, chromedp.Click(`#node-list .node-expand-button[data-expand-node-id="`+nodeID+`"]`, chromedp.ByQuery)); err != nil {
		t.Fatalf("expand node %s: %v", nodeID, err)
	}
}

func clickContainer(t *testing.T, ctx context.Context, nodeID, containerID string) {
	t.Helper()
	selector := `#node-list .container-item[data-node-id="` + nodeID + `"][data-container-id="` + containerID + `"]`
	if err := chromedp.Run(ctx, chromedp.Click(selector, chromedp.ByQuery)); err != nil {
		t.Fatalf("click container %s/%s: %v", nodeID, containerID, err)
	}
}

// TestE2EContainerClicksDoNotWidenNodeSelection guards the reported
// "现在是单选，但是还是有两个容器被选中，右侧也显示已选 2 个 Dozzle 节点".
//
// Container scopes are additive inside a node, but the click handler derived
// the NODE selection from every node owning a selected container -- without
// consulting the switch. Picking a container under one node and then another
// under a second node therefore highlighted two nodes while the switch still
// said single-select.
//
// With the switch on, spanning nodes is legitimate, so the second half of the
// test locks that in too.
func TestE2EContainerClicksDoNotWidenNodeSelection(t *testing.T) {
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
		nodes: []Node{
			{ID: "node-a", Name: "溯帆", URL: "http://124.174.71.198:8099", Status: "connected", Initial: "溯",
				Containers: []containerInfo{{ID: "api", Name: "api", State: "running"}}},
			{ID: "node-b", Name: "掌门人", URL: "https://115.190.152.177:8999", Status: "connected", Initial: "掌",
				Containers: []containerInfo{{ID: "web", Name: "web", State: "running"}}},
		},
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
		chromedp.WaitVisible(`#node-list`, chromedp.ByQuery),
		chromedp.WaitVisible(`#node-list .node-item[data-node-id="node-a"]`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("open page: %v", err)
	}

	// --- Switch off (the default): a container click must not widen the node
	// selection past one node.
	ensureNodeExpanded(t, ctx, "node-a")
	clickContainer(t, ctx, "node-a", "api")
	ensureNodeExpanded(t, ctx, "node-b")
	clickContainer(t, ctx, "node-b", "web")

	single := readContainerScopeView(t, ctx)
	if single.ToggleOn {
		t.Fatalf("switch should still be off at this point (%+v)", single)
	}
	if len(single.Active) != 1 {
		t.Fatalf("switch is off but %d nodes are highlighted: %v (%+v)", len(single.Active), single.Active, single)
	}
	if single.Active[0] != "node-b" {
		t.Fatalf("expected the container click to move the selection to node-b, got %v (%+v)", single.Active, single)
	}
	// The caption is the second half of the report: it must not claim two nodes.
	// Copy follows the UI language, so compare against the page's dictionary.
	multiCaption := strings.ReplaceAll(i18nLabel(t, ctx, "stream.captionNodes"), "{count}", "2")
	if strings.Contains(single.Caption, multiCaption) {
		t.Fatalf("detail caption still reports a multi-node selection: %q (%+v)", single.Caption, single)
	}
	// And the earlier node's container scope must have been dropped, not left
	// silently scoping the stream to a node that is no longer selected.
	for _, key := range single.Containers {
		if strings.HasPrefix(key, "node-a::") {
			t.Fatalf("container scope from the deselected node survived: %v (%+v)", single.Containers, single)
		}
	}

	// --- Switch on: spanning nodes from container clicks is legitimate.
	clickMultiSelectToggle(t, ctx)
	ensureNodeExpanded(t, ctx, "node-a")
	clickContainer(t, ctx, "node-a", "api")

	multi := readContainerScopeView(t, ctx)
	if !multi.ToggleOn {
		t.Fatalf("switch should be on here (%+v)", multi)
	}
	if len(multi.Active) != 2 {
		t.Fatalf("with the switch on, container clicks should span both nodes, got %v (%+v)", multi.Active, multi)
	}
}

// TestE2EStaleContainerScopeIsPrunedOnSync guards a container scope outliving
// its node.
//
// syncGoBackend() prunes selectedNodes and expandedNodes whenever the node list
// changes, but left selectedContainers untouched. Since
// logsForActiveContainer() filters the stream by those keys, a scope pointing at
// a node that is gone silently blanks the log stream with no visible cause.
//
// The node is removed through the API rather than the UI on purpose: unbindNode()
// already prunes client-side, so going through it would not exercise the sync.
// This is the case only the sync can catch -- a node that disappears while the
// page is open, because it was unbound from another tab or dropped from a
// hand-edited config.
func TestE2EStaleContainerScopeIsPrunedOnSync(t *testing.T) {
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
		nodes: []Node{
			{ID: "node-a", Name: "溯帆", URL: "http://124.174.71.198:8099", Status: "connected", Initial: "溯",
				Containers: []containerInfo{{ID: "api", Name: "api", State: "running"}}},
			{ID: "node-b", Name: "掌门人", URL: "https://115.190.152.177:8999", Status: "connected", Initial: "掌",
				Containers: []containerInfo{{ID: "web", Name: "web", State: "running"}}},
		},
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
		chromedp.WaitVisible(`#node-list`, chromedp.ByQuery),
		chromedp.WaitVisible(`#node-list .node-item[data-node-id="node-a"]`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("open page: %v", err)
	}

	// Scope one container under each node. The switch has to be on for the second
	// scope to add to the first rather than replace it.
	clickMultiSelectToggle(t, ctx)
	ensureNodeExpanded(t, ctx, "node-a")
	clickContainer(t, ctx, "node-a", "api")
	ensureNodeExpanded(t, ctx, "node-b")
	clickContainer(t, ctx, "node-b", "web")

	var seeded []string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`state.selectedContainers`, &seeded)); err != nil {
		t.Fatalf("read seeded scopes: %v", err)
	}
	if len(seeded) != 2 {
		t.Fatalf("expected both container scopes to be selected, got %v", seeded)
	}

	// Remove node-b behind the page's back. unbindNode() is deliberately NOT
	// used, so the only thing that can clean up the scope is the sync.
	req, err := http.NewRequest(http.MethodDelete, web.URL+"/api/nodes/node-b", nil)
	if err != nil {
		t.Fatalf("build delete request: %v", err)
	}
	resp, err := web.Client().Do(req)
	if err != nil {
		t.Fatalf("delete node-b: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		t.Fatalf("delete node-b returned %d, want a 2xx", resp.StatusCode)
	}

	// The sync re-runs every 3s, so poll until the stale scope is gone.
	var scopes []string
	deadline := time.Now().Add(15 * time.Second)
	for {
		if err := chromedp.Run(ctx, chromedp.Evaluate(`state.selectedContainers`, &scopes)); err != nil {
			t.Fatalf("read container scopes: %v", err)
		}
		stale := false
		for _, key := range scopes {
			if strings.HasPrefix(key, "node-b::") {
				stale = true
			}
		}
		if !stale || time.Now().After(deadline) {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	for _, key := range scopes {
		if strings.HasPrefix(key, "node-b::") {
			t.Fatalf("container scope outlived its node: %v -- it would filter the stream to nothing", scopes)
		}
	}
	// The surviving node's own scope must not be swept up with it.
	found := false
	for _, key := range scopes {
		if key == "node-a::api" {
			found = true
		}
	}
	if !found {
		t.Fatalf("pruning removed the live node's scope too: %v", scopes)
	}
}

// selectionInvariantView is the pair of selections the invariant relates.
type selectionInvariantView struct {
	Nodes      []string `json:"nodes"`
	Containers []string `json:"containers"`
	ToggleOn   bool     `json:"toggleOn"`
}

func readSelectionInvariant(t *testing.T, ctx context.Context) selectionInvariantView {
	t.Helper()
	var raw string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`JSON.stringify({
		nodes: state.selectedNodes,
		containers: state.selectedContainers,
		toggleOn: state.multiSelect,
	})`, &raw)); err != nil {
		t.Fatalf("read selection invariant: %v", err)
	}
	var v selectionInvariantView
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("decode selection invariant %q: %v", raw, err)
	}
	return v
}

// assertSelectionInvariant checks the two rules the sidebar selection must obey
// after ANY interaction:
//
//  1. the switch is the authority -- with it off, at most one node is selected;
//  2. a container scope belongs to a selected node, because
//     logsForActiveContainer() filters the stream by those keys and a scope
//     whose node is not selected would blank the log stream silently.
//
// Four separate bugs have landed in this feature, each one a different code path
// forgetting one of these. Asserting after every step of a long mixed sequence
// catches a path that the targeted tests do not enumerate.
func assertSelectionInvariant(t *testing.T, ctx context.Context, step string) {
	t.Helper()
	v := readSelectionInvariant(t, ctx)
	if !v.ToggleOn && len(v.Nodes) > 1 {
		t.Fatalf("after %s: switch is off but %d nodes are selected: %v", step, len(v.Nodes), v.Nodes)
	}
	selected := make(map[string]bool, len(v.Nodes))
	for _, id := range v.Nodes {
		selected[id] = true
	}
	for _, key := range v.Containers {
		node := key
		if i := strings.Index(key, "::"); i >= 0 {
			node = key[:i]
		}
		if !selected[node] {
			t.Fatalf("after %s: container scope %q belongs to a node that is not selected (%v)", step, key, v.Nodes)
		}
	}
}

// TestE2ESelectionInvariantHoldsAcrossInteractions drives a long sequence of
// node clicks, container clicks and switch toggles, asserting both invariants
// after every single step.
func TestE2ESelectionInvariantHoldsAcrossInteractions(t *testing.T) {
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
		nodes: []Node{
			{ID: "node-a", Name: "溯帆", URL: "http://124.174.71.198:8099", Status: "connected", Initial: "溯",
				Containers: []containerInfo{{ID: "api", Name: "api", State: "running"}, {ID: "web", Name: "web", State: "running"}}},
			{ID: "node-b", Name: "掌门人", URL: "https://115.190.152.177:8999", Status: "connected", Initial: "掌",
				Containers: []containerInfo{{ID: "gateway", Name: "gateway", State: "running"}, {ID: "db", Name: "db", State: "running"}}},
			{ID: "node-c", Name: "匹配AI", URL: "http://115.190.120.65:8099", Status: "connected", Initial: "匹",
				Containers: []containerInfo{{ID: "worker", Name: "worker", State: "running"}}},
		},
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
		chromedp.WaitVisible(`#node-list`, chromedp.ByQuery),
		chromedp.WaitVisible(`#node-list .node-item[data-node-id="node-a"]`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("open page: %v", err)
	}

	expandAll := func(ids ...string) {
		for _, id := range ids {
			ensureNodeExpanded(t, ctx, id)
		}
	}
	expandAll("node-a", "node-b", "node-c")

	// --- Switch OFF: at most one node may ever be selected.
	steps := []struct {
		label string
		run   func()
	}{
		{"click node-a", func() { clickNode(t, ctx, "node-a") }},
		{"click container node-a/api", func() { clickContainer(t, ctx, "node-a", "api") }},
		{"click container node-a/web", func() { clickContainer(t, ctx, "node-a", "web") }},
		{"click node-b", func() { clickNode(t, ctx, "node-b") }},
		{"click container node-b/gateway", func() { clickContainer(t, ctx, "node-b", "gateway") }},
		{"click container node-c/worker", func() { clickContainer(t, ctx, "node-c", "worker") }},
		{"click node-c", func() { clickNode(t, ctx, "node-c") }},
		{"click container node-a/api", func() { clickContainer(t, ctx, "node-a", "api") }},
		{"click node-a", func() { clickNode(t, ctx, "node-a") }},
		{"click container node-b/db", func() { clickContainer(t, ctx, "node-b", "db") }},
		{"click container node-b/db again", func() { clickContainer(t, ctx, "node-b", "db") }},
	}
	for _, step := range steps {
		step.run()
		assertSelectionInvariant(t, ctx, step.label)
	}

	// --- Switch ON: several nodes are allowed, but a container scope must still
	// belong to a selected node.
	clickMultiSelectToggle(t, ctx)
	assertSelectionInvariant(t, ctx, "turn the switch on")

	multiSteps := []struct {
		label string
		run   func()
	}{
		{"click node-a", func() { clickNode(t, ctx, "node-a") }},
		{"click node-b", func() { clickNode(t, ctx, "node-b") }},
		{"click container node-c/worker", func() { clickContainer(t, ctx, "node-c", "worker") }},
		{"click container node-a/api", func() { clickContainer(t, ctx, "node-a", "api") }},
		{"click node-a (deselect)", func() { clickNode(t, ctx, "node-a") }},
		{"click container node-a/web", func() { clickContainer(t, ctx, "node-a", "web") }},
		{"click node-b (deselect)", func() { clickNode(t, ctx, "node-b") }},
	}
	for _, step := range multiSteps {
		step.run()
		assertSelectionInvariant(t, ctx, step.label)
	}

	// --- Turning the switch off must collapse a multi-node selection.
	clickMultiSelectToggle(t, ctx)
	assertSelectionInvariant(t, ctx, "turn the switch back off")
}

// TestE2ECommandPopupsCanBeDragged checks that the file and server popups move
// with their heading, so they can be pushed off whatever the user wants to drop
// on, that they cannot be dragged off the screen, and that closing one sends it
// back to its home position.
func TestE2ECommandPopupsCanBeDragged(t *testing.T) {
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
		chromedp.WindowSize(1600, 1000),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocatorOptions...)
	t.Cleanup(cancelAlloc)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithErrorf(func(string, ...any) {}))
	t.Cleanup(cancel)

	if err := chromedp.Run(ctx,
		chromedp.Navigate(web.URL),
		chromedp.WaitVisible(`#node-list`, chromedp.ByQuery),
		chromedp.Evaluate(`setView('commands')`, nil),
		chromedp.WaitVisible(`.commands-columns`, chromedp.ByQuery),
		chromedp.Click(`#command-dock-files`, chromedp.ByQuery),
		chromedp.WaitVisible(`#command-file-drop-zone`, chromedp.ByQuery),
		chromedp.Sleep(400*time.Millisecond),
	); err != nil {
		t.Fatalf("open file popup: %v", err)
	}

	type rect struct {
		Left, Top, Right, Bottom float64
		HeadX, HeadY             float64
		MenuBottom               float64
		ViewportW, ViewportH     float64
	}
	measure := func() rect {
		t.Helper()
		var raw string
		if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
			const popup = document.querySelector('#command-popup-files').getBoundingClientRect();
			const title = document.querySelector('#command-popup-files h2').getBoundingClientRect();
			return JSON.stringify({
				Left: popup.left, Top: popup.top, Right: popup.right, Bottom: popup.bottom,
				HeadX: title.left + 20, HeadY: title.top + title.height / 2,
				MenuBottom: document.getElementById('menubar').getBoundingClientRect().bottom,
				ViewportW: window.innerWidth, ViewportH: window.innerHeight,
			});
		})()`, &raw)); err != nil {
			t.Fatalf("measure popup: %v", err)
		}
		var r rect
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			t.Fatalf("decode popup rect %q: %v", raw, err)
		}
		return r
	}
	drag := func(from rect, dx, dy float64) {
		t.Helper()
		if err := chromedp.Run(ctx,
			chromedp.MouseEvent(input.MouseMoved, from.HeadX, from.HeadY),
			chromedp.MouseEvent(input.MousePressed, from.HeadX, from.HeadY, chromedp.ButtonLeft),
			chromedp.MouseEvent(input.MouseMoved, from.HeadX+dx/2, from.HeadY+dy/2, chromedp.ButtonLeft),
			chromedp.MouseEvent(input.MouseMoved, from.HeadX+dx, from.HeadY+dy, chromedp.ButtonLeft),
			chromedp.MouseEvent(input.MouseReleased, from.HeadX+dx, from.HeadY+dy, chromedp.ButtonLeft),
		); err != nil {
			t.Fatalf("drag popup: %v", err)
		}
	}
	near := func(got, want float64) bool { return got-want < 2 && want-got < 2 }

	before := measure()
	drag(before, -200, -150)
	after := measure()
	if !near(after.Left-before.Left, -200) || !near(after.Top-before.Top, -150) {
		t.Fatalf("popup moved by (%.1f, %.1f), want (-200, -150): before=%+v after=%+v",
			after.Left-before.Left, after.Top-before.Top, before, after)
	}

	// Dragging far past every edge stops at the window and the menu bar.
	drag(after, -5000, -5000)
	corner := measure()
	if corner.Left < -0.5 || corner.Top < corner.MenuBottom-0.5 {
		t.Fatalf("popup left the screen at the top-left: %+v", corner)
	}
	drag(corner, 9000, 9000)
	far := measure()
	if far.Right > far.ViewportW+0.5 || far.Bottom > far.ViewportH+0.5 {
		t.Fatalf("popup left the screen at the bottom-right: %+v", far)
	}

	// Closing the popup puts it back where it started when it is reopened.
	var reopened rect
	if err := chromedp.Run(ctx,
		chromedp.Click(`#command-dock-files`, chromedp.ByQuery),
		chromedp.Click(`#command-dock-files`, chromedp.ByQuery),
		chromedp.WaitVisible(`#command-file-drop-zone`, chromedp.ByQuery),
		chromedp.Sleep(300*time.Millisecond),
	); err != nil {
		t.Fatalf("reopen popup: %v", err)
	}
	reopened = measure()
	if !near(reopened.Left, before.Left) || !near(reopened.Top, before.Top) {
		t.Fatalf("reopened popup is not back at its home position: home=%+v reopened=%+v", before, reopened)
	}
}

// TestE2EFlowListKeepsItsWidthWithGroupsOpen guards the flow list against being
// squeezed to nothing when the groups drawer opens inside the board: the drawer
// takes a fixed width, so the board must be given enough of the row for the flow
// names to stay readable and the cards to stay inside the panel.
func TestE2EFlowListKeepsItsWidthWithGroupsOpen(t *testing.T) {
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
		// The window size the layout was reported at: a 975px column row.
		chromedp.WindowSize(1440, 900),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocatorOptions...)
	t.Cleanup(cancelAlloc)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithErrorf(func(string, ...any) {}))
	t.Cleanup(cancel)

	seed := `(() => {
		const servers = [{ id: 'srv-0', name: '溯帆服务器', host: '124.174.71.198', port: '22', user: 'root', auth: 'password', secret: 'x', connectionStatus: 'success' }];
		const flows = [];
		for (let i = 0; i < 3; i++) {
			flows.push({ id: 'flow-' + i, name: '封包更新分后台' + i, lines: [{ id: 'l-' + i, text: 'ls' }], bindings: [{ serverId: 'srv-0' }] });
		}
		const request = new XMLHttpRequest();
		request.open('PUT', '/api/commands/state', false);
		request.setRequestHeader('Content-Type', 'application/json');
		request.send(JSON.stringify({ flows, servers }));
		if (request.status !== 200) throw new Error('seed commands.json: ' + request.status);
		return 'ok';
	})()`

	var raw string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(web.URL),
		chromedp.WaitVisible(`#node-list`, chromedp.ByQuery),
		chromedp.Evaluate(seed, nil),
		chromedp.Reload(),
		chromedp.WaitVisible(`#node-list`, chromedp.ByQuery),
		chromedp.Evaluate(`setView('commands')`, nil),
		chromedp.WaitVisible(`.commands-columns`, chromedp.ByQuery),
		chromedp.Evaluate(`setCommandGroupsOpen(true)`, nil),
		chromedp.Sleep(400*time.Millisecond),
		chromedp.Evaluate(`(() => {
			const main = document.querySelector('.commands-board-main');
			const list = document.querySelector('#commands-flow-list');
			const name = document.querySelector('.command-flow-card .command-flow-main strong');
			const editor = document.querySelector('#commands-editor');
			return JSON.stringify({
				mainW: main.getBoundingClientRect().width,
				editorW: editor.getBoundingClientRect().width,
				listScrollW: list.scrollWidth, listClientW: list.clientWidth,
				nameScrollW: name.scrollWidth, nameClientW: name.clientWidth,
				cards: document.querySelectorAll('.command-flow-card').length,
			});
		})()`, &raw),
	); err != nil {
		t.Fatalf("open groups drawer: %v", err)
	}
	var v struct {
		MainW       float64 `json:"mainW"`
		EditorW     float64 `json:"editorW"`
		ListScrollW float64 `json:"listScrollW"`
		ListClientW float64 `json:"listClientW"`
		NameScrollW float64 `json:"nameScrollW"`
		NameClientW float64 `json:"nameClientW"`
		Cards       int     `json:"cards"`
	}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("decode groups layout %q: %v", raw, err)
	}
	if v.Cards != 3 {
		t.Fatalf("expected the 3 seeded flows, got %d (%+v)", v.Cards, v)
	}
	if v.MainW < 340 {
		t.Fatalf("flow list is only %.0fpx wide with the groups drawer open, want at least 340 (%+v)", v.MainW, v)
	}
	if v.EditorW < 300 {
		t.Fatalf("editor was squeezed to %.0fpx by the open groups drawer (%+v)", v.EditorW, v)
	}
	if v.ListScrollW > v.ListClientW+1 {
		t.Fatalf("flow cards overflow the list horizontally: scrollWidth=%.0f clientWidth=%.0f (%+v)", v.ListScrollW, v.ListClientW, v)
	}
	if v.NameScrollW > v.NameClientW+1 {
		t.Fatalf("flow name is truncated at this width: scrollWidth=%.0f clientWidth=%.0f (%+v)", v.NameScrollW, v.NameClientW, v)
	}
}

// TestE2ETerminalPage drives the server terminal page against an in-process SSH
// server: the rail labels, connecting, typing into the shell, and the file
// browser (file/folder uploads, directory drops, new folder, open, delete).
func TestE2ETerminalPage(t *testing.T) {
	browserPath := firstExistingPath(
		`C:/Program Files\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		`C:/Program Files\Microsoft\Edge\Application\msedge.exe`,
		`C:/Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	)
	if browserPath == "" {
		t.Skip("Chrome or Edge is required for UI end-to-end tests")
	}
	t.Cleanup(closeAllTermSessions)
	t.Setenv("LOG_AGENT_CONFIG_DIR", t.TempDir())
	*configDirCache() = configDirState{}
	host, port, fingerprint := startFakeSSHServer(t)

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
		chromedp.WindowSize(1440, 900),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocatorOptions...)
	t.Cleanup(cancelAlloc)
	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithErrorf(func(string, ...any) {}))
	t.Cleanup(cancel)

	// prompt() asks for the name of a new folder; confirm() guards deletion.
	chromedp.ListenTarget(ctx, func(ev any) {
		if dialog, ok := ev.(*page.EventJavascriptDialogOpening); ok {
			go func() {
				_ = chromedp.Run(ctx, page.HandleJavaScriptDialog(true).WithPromptText("newdir"))
			}()
			_ = dialog
		}
	})

	seed := `(() => {
		const servers = [{ id: 'srv-0', name: '测试服务器', host: '` + host + `', port: '` + port + `', user: 'tester', auth: 'password', secret: 'pw', connectionStatus: 'success', hostFingerprint: '` + fingerprint + `' }];
		const flows = [{ id: 'flow-0', name: '检查磁盘', lines: [{ id: 'l-0', type: 'command', text: 'df -h' }], bindings: [] }];
		const request = new XMLHttpRequest();
		request.open('PUT', '/api/commands/state', false);
		request.setRequestHeader('Content-Type', 'application/json');
		request.send(JSON.stringify({ flows, servers }));
		if (request.status !== 200) throw new Error('seed commands.json: ' + request.status);
		return 'ok';
	})()`
	wait := func(expr, what string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			var ok bool
			if err := chromedp.Run(ctx, chromedp.Evaluate(expr, &ok)); err == nil && ok {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		var dump string
		_ = chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#terminal-view').innerText.slice(0, 1200)`, &dump))
		t.Fatalf("timed out waiting for %s; page text: %q", what, dump)
	}

	upload := filepathJoinTemp(t, "upload-me.txt", "uploaded from the page\n")
	if err := chromedp.Run(ctx,
		chromedp.Navigate(web.URL),
		chromedp.WaitVisible(`#node-list`, chromedp.ByQuery),
		chromedp.Evaluate(seed, nil),
		chromedp.Reload(),
		chromedp.WaitVisible(`#node-list`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("load page: %v", err)
	}

	// The old page is "指令集组" now and the SSH client is "服务器".
	var labels string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`Array.from(document.querySelectorAll('.rail .nav-item')).map((b) => b.dataset.view + '=' + b.textContent.replace(/[0-9]+$/, '').trim()).join('|')`, &labels)); err != nil {
		t.Fatal(err)
	}
	// The browser here runs in English; the Chinese UI says 指令集组 / 服务器.
	if !strings.Contains(labels, "commands=▤Command Groups") || !strings.Contains(labels, "terminal=>_Servers") {
		t.Fatalf("rail labels = %q, want Command Groups for commands and Servers for terminal", labels)
	}
	var commandsTitle string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#commands-view h1').textContent`, &commandsTitle)); err != nil || commandsTitle != "Command Groups" {
		t.Fatalf("commands page title = %q (%v), want Command Groups", commandsTitle, err)
	}

	if err := chromedp.Run(ctx,
		chromedp.Click(`.nav-item[data-view="terminal"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`#term-server-select`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("open terminal page: %v", err)
	}
	wait(`document.querySelector('#term-server-select').value === 'srv-0'`, "the server list")
	// Page height: the terminal page must fit the window.
	var overflow float64
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.documentElement.scrollHeight - window.innerHeight`, &overflow)); err != nil || overflow > 2 {
		t.Fatalf("terminal page grew a scrollbar: overflow=%.1f (%v)", overflow, err)
	}

	if err := chromedp.Run(ctx, chromedp.Click(`#term-connect`, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	wait(`!!document.querySelector('.term-tab.open')`, "the session tab to open")
	wait(`(document.querySelector('.term-session.active .xterm-rows') || {innerText: ''}).innerText.includes('ready$')`, "the shell prompt")

	// The terminal must fill its panel: xterm starts at 80x24 and only grows once
	// it has been fitted, which left a dead strip under the last row.
	wait(`(() => {
		const session = document.querySelector('.term-session.active').getBoundingClientRect();
		const screen = document.querySelector('.term-session.active .xterm-screen').getBoundingClientRect();
		return session.height - screen.height >= 0 && session.height - screen.height < 24 && screen.width > session.width * 0.9;
	})()`, "the terminal to fill its panel")
	// xterm draws its own overlay scrollbar; the native one under it showed up as
	// a small white mark in the corner, so the viewport must not have one.
	wait(`(() => { const v = document.querySelector('.term-session.active .xterm-viewport'); return v.offsetWidth === v.clientWidth; })()`, "the native scrollbar to stay hidden")
	// SendKeys fires both a key event and an input event for each character, which
	// xterm answers twice; inserting the text is what typing into the focused
	// textarea amounts to.
	if err := chromedp.Run(ctx,
		chromedp.Focus(`.term-session.active .xterm-helper-textarea`, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error { return input.InsertText("hello-terminal\r").Do(ctx) }),
	); err != nil {
		t.Fatalf("type into the terminal: %v", err)
	}
	wait(`document.querySelector('.term-session.active .xterm-rows').innerText.includes('hello-terminal')`, "typed text echoed by the shell")

	// Files: the in-memory SFTP root starts empty.
	wait(`document.querySelector('#term-path').value === '/' && document.querySelector('#term-file-rows .term-empty-row') !== null`, "an empty root listing")
	if err := chromedp.Run(ctx, chromedp.SetUploadFiles(`#term-upload-input`, []string{upload}, chromedp.ByQuery)); err != nil {
		t.Fatalf("choose a file: %v", err)
	}
	wait(`!!document.querySelector('#term-file-rows tr[data-name="upload-me.txt"]')`, "the uploaded file in the listing")

	if err := chromedp.Run(ctx, chromedp.Click(`#term-mkdir`, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	wait(`!!document.querySelector('#term-file-rows tr[data-name="newdir"]')`, "the new folder in the listing")
	// Folders sort before files.
	var first string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#term-file-rows tr[data-name]').dataset.name`, &first)); err != nil || first != "newdir" {
		t.Fatalf("first row = %q (%v), want the folder first", first, err)
	}

	// Select the file and delete it (the confirm dialog is accepted above).
	if err := chromedp.Run(ctx,
		chromedp.Click(`#term-file-rows tr[data-name="upload-me.txt"]`, chromedp.ByQuery),
		chromedp.Click(`#term-delete`, chromedp.ByQuery),
	); err != nil {
		t.Fatal(err)
	}
	wait(`!document.querySelector('#term-file-rows tr[data-name="upload-me.txt"]')`, "the file to disappear after deleting it")

	// Opening the folder moves the path and the tree.
	if err := chromedp.Run(ctx, chromedp.DoubleClick(`#term-file-rows tr[data-name="newdir"]`, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	wait(`document.querySelector('#term-path').value === '/newdir' && !!document.querySelector('.term-tree-row.active[data-path="/newdir"]')`, "the folder to open")
	if err := chromedp.Run(ctx, chromedp.Click(`#term-up`, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	wait(`document.querySelector('#term-path').value === '/'`, "the parent folder")

	// Choosing a folder keeps the root name, nested paths and duplicate basenames.
	folder := filepath.Join(t.TempDir(), "picked-folder")
	if err := os.MkdirAll(filepath.Join(folder, "子目录"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"same.txt": "root content", "子目录/same.txt": "nested content"} {
		if err := os.WriteFile(filepath.Join(folder, filepath.FromSlash(name)), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := chromedp.Run(ctx, chromedp.SetUploadFiles(`#term-upload-folder-input`, []string{folder}, chromedp.ByQuery)); err != nil {
		t.Fatalf("choose a folder: %v", err)
	}
	wait(`!!document.querySelector('#term-file-rows tr[data-name="picked-folder"]')`, "the selected folder to finish uploading")

	termSessions.mu.Lock()
	var sessionID string
	for id := range termSessions.sessions {
		sessionID = id
	}
	termSessions.mu.Unlock()
	readRemote := func(action, path string) []byte {
		t.Helper()
		response, err := http.Get(web.URL + "/api/term/fs/" + action + "?" + url.Values{"id": {sessionID}, "path": {path}}.Encode())
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("read remote %s: HTTP %d, %s (%v)", path, response.StatusCode, data, err)
		}
		return data
	}
	for path, want := range map[string]string{"/picked-folder/same.txt": "root content", "/picked-folder/子目录/same.txt": "nested content"} {
		if got := string(readRemote("download", path)); got != want {
			t.Fatalf("remote %s = %q, want %q", path, got, want)
		}
	}

	// A dropped directory reader may return several batches. Empty directories
	// and loose files in the same drop must also reach the remote file system.
	dropFolder := `(() => {
		const file = (name, content) => ({ name, isFile: true, file: (resolve) => resolve(new File([content], name)) });
		const directory = (name, batches) => ({ name, isDirectory: true, createReader: () => {
			let index = 0;
			return { readEntries: (resolve) => setTimeout(() => resolve(batches[index++] || []), 5) };
		}});
		const first = Array.from({length: 100}, (_, i) => file('file-' + i + '.txt', String(i)));
		const root = directory('dropped-folder', [first, [file('after-100.txt', 'last batch'), directory('empty', []), directory('nested', [[file('zero.txt', '')]])]]);
		const loose = new File(['loose content'], 'loose.txt');
		const transfer = new DataTransfer();
		transfer.items.add(loose);
		Object.defineProperty(transfer, 'items', { value: [
			{kind: 'file', webkitGetAsEntry: () => root, getAsFile: () => null},
			{kind: 'file', webkitGetAsEntry: () => null, getAsFile: () => loose}
		] });
		document.querySelector('#term-table-wrap').dispatchEvent(new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer: transfer }));
	})()`
	if err := chromedp.Run(ctx, chromedp.Evaluate(dropFolder, nil)); err != nil {
		t.Fatalf("drop a folder: %v", err)
	}
	wait(`!!document.querySelector('#term-file-rows tr[data-name="dropped-folder"]') && !!document.querySelector('#term-file-rows tr[data-name="loose.txt"]')`, "the dropped folder and loose file")
	for path, want := range map[string]string{"/dropped-folder/file-99.txt": "99", "/dropped-folder/after-100.txt": "last batch", "/dropped-folder/nested/zero.txt": "", "/loose.txt": "loose content"} {
		if got := string(readRemote("download", path)); got != want {
			t.Fatalf("remote %s = %q, want %q", path, got, want)
		}
	}
	var emptyDir termListResponse
	if err := json.Unmarshal(readRemote("list", "/dropped-folder/empty"), &emptyDir); err != nil || len(emptyDir.Entries) != 0 {
		t.Fatalf("empty directory was not preserved: %+v (%v)", emptyDir, err)
	}

	// Reading a dropped directory is asynchronous. Navigating meanwhile must not
	// change its upload destination or move the user back after completion.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		const transfer = new DataTransfer();
		transfer.items.add(new File([], 'snapshot-folder'));
		const root = { name: 'snapshot-folder', isDirectory: true, createReader: () => {
			let sent = false;
			return { readEntries: (resolve) => setTimeout(() => {
				resolve(sent ? [] : [{ name: 'target.txt', isFile: true, file: (done) => done(new File(['original destination'], 'target.txt')) }]);
				sent = true;
			}, 100) };
		}};
		Object.defineProperty(transfer, 'items', { value: [{kind: 'file', webkitGetAsEntry: () => root, getAsFile: () => null}] });
		document.querySelector('#term-table-wrap').dispatchEvent(new DragEvent('drop', {bubbles: true, cancelable: true, dataTransfer: transfer}));
		const path = document.querySelector('#term-path');
		path.value = '/newdir';
		path.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true}));
	})()`, nil)); err != nil {
		t.Fatal(err)
	}
	wait(`document.querySelector('#term-path').value === '/newdir' && document.querySelector('#term-status').textContent === 'Uploaded 1 file(s) and 1 folder(s)'`, "uploading to the original directory while browsing another")
	if got := string(readRemote("download", "/snapshot-folder/target.txt")); got != "original destination" {
		t.Fatalf("upload destination changed: %q", got)
	}

	// Directory failures stop the batch and remain visible rather than being
	// replaced with a success message for earlier files.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		const original = window.fetch;
		window.__blockedUploads = 0;
		window.fetch = (input, init) => {
			const target = String(input);
			if (target.includes('/fs/mkdir?') && target.includes('denied-folder')) return Promise.resolve(new Response(JSON.stringify({error: 'upload_failed', detail: 'blocked fixture'}), {status: 403}));
			if (target.includes('/fs/upload?') && target.includes('denied-folder')) window.__blockedUploads++;
			return original(input, init);
		};
		const file = new File(['not uploaded'], 'blocked.txt');
		Object.defineProperty(file, 'webkitRelativePath', {value: 'denied-folder/blocked.txt'});
		const transfer = new DataTransfer(); transfer.items.add(file);
		const input = document.querySelector('#term-upload-folder-input');
		input.files = transfer.files;
		input.dispatchEvent(new Event('change', {bubbles: true}));
	})()`, nil)); err != nil {
		t.Fatal(err)
	}
	wait(`document.querySelector('#term-status').textContent.includes('blocked fixture') && window.__blockedUploads === 0`, "the folder creation failure without uploading child files")

	// Directory entries and their tree children remain visible, even after a
	// delayed navigation response or a transient tree-listing failure.
	for _, directory := range []string{"/home", "/home/deploy", "/home/ruoyi", "/home/stack", "/home/stack/data"} {
		response, err := http.Post(web.URL+"/api/term/fs/mkdir?"+url.Values{"id": {sessionID}, "path": {directory}}.Encode(), "application/octet-stream", nil)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("fixture mkdir %s: %d", directory, response.StatusCode)
		}
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => { const path=document.querySelector('#term-path');path.value='/home';path.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true})); })()`, nil)); err != nil {
		t.Fatal(err)
	}
	wait(`['deploy','ruoyi','stack'].every(name=>document.querySelector('#term-file-rows tr[data-name="'+name+'"]') && document.querySelector('.term-tree-row[data-path="/home/'+name+'"]'))`, "all home directories in the listing and tree")
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		const original=window.fetch; window.__beforeDirectoryProbe=original;
		window.fetch=(input,init)=>{ const url=new URL(String(input),location.href);
			if(url.pathname==='/api/term/fs/list' && url.searchParams.get('path')==='/late-listing') return new Promise(resolve=>{ window.__releaseDirectoryProbe=()=>resolve(new Response(JSON.stringify({path:'/late-listing',entries:[]}),{status:200})); });
			return original(input,init);
		};
		const path=document.querySelector('#term-path'); path.value='/late-listing';path.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true}));
		path.value='/home/ruoyi';path.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true}));
	})()`, nil)); err != nil {
		t.Fatal(err)
	}
	wait(`document.querySelector('#term-path').value==='/home/ruoyi' && !!document.querySelector('.term-tree-row.active[data-path="/home/ruoyi"]')`, "new navigation completed before the old response")
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.__releaseDirectoryProbe();window.fetch=window.__beforeDirectoryProbe`, nil), chromedp.Sleep(150*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	var latestDirectory bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#term-path').value==='/home/ruoyi' && !!document.querySelector('.term-tree-row.active[data-path="/home/ruoyi"]')`, &latestDirectory)); err != nil || !latestDirectory {
		t.Fatalf("stale directory response replaced the active folder: %v", err)
	}
	if err := chromedp.Run(ctx, chromedp.Click(`.term-tree-row[data-path="/home"] span`, chromedp.ByQuery), chromedp.Evaluate(`(() => {const original=window.fetch;window.__treeProbeCount=0;window.__beforeTreeProbe=original;window.fetch=(input,init)=>{const url=new URL(String(input),location.href);if(url.pathname==='/api/term/fs/list' && url.searchParams.get('path')==='/home/stack' && ++window.__treeProbeCount===1)return Promise.resolve(new Response(JSON.stringify({error:'temporary fixture failure'}),{status:503}));return original(input,init);};})()`, nil), chromedp.Click(`[data-toggle="/home/stack"]`, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	wait(`window.__treeProbeCount===1 && document.querySelector('[data-toggle="/home/stack"]').textContent==='▾'`, "transient tree read failure")
	if err := chromedp.Run(ctx, chromedp.Click(`[data-toggle="/home/stack"]`, chromedp.ByQuery), chromedp.Click(`[data-toggle="/home/stack"]`, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	wait(`window.__treeProbeCount===2 && !!document.querySelector('.term-tree-row[data-path="/home/stack/data"]')`, "failed tree listing can be retried instead of staying empty")
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.fetch=window.__beforeTreeProbe;window.__savedTerminalServers=state.commandServers.slice();state.commandServers.push({...state.commandServers[0],id:'srv-other',name:'Other session'});renderTerminalView();document.querySelector('#term-server-select').value='srv-other'`, nil), chromedp.Click(`#term-connect`, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	wait(`!!document.querySelector('.term-tab.open[data-server-id="srv-other"]')`, "second terminal session")
	if err := chromedp.Run(ctx, chromedp.Click(`.term-tab[data-server-id="srv-0"] span`, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	wait(`document.querySelector('#term-server-select').value==='srv-0' && document.querySelector('#term-file-source').textContent.includes(state.commandServers[0].host) && document.querySelector('#term-path').value==='/home'`, "server selection and file source follow the active session")
	var snapshotRetained bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {const server=state.commandServers[0],host=server.host;server.host='changed.invalid';renderTerminalView();const unchanged=!document.querySelector('#term-file-source').textContent.includes('changed.invalid');server.host=host;return unchanged;})()`, &snapshotRetained)); err != nil || !snapshotRetained {
		t.Fatalf("editing a saved server relabeled an existing connection: %v", err)
	}
	if err := chromedp.Run(ctx, chromedp.Click(`.term-tab[data-server-id="srv-other"] .term-tab-close`, chromedp.ByQuery), chromedp.Evaluate(`state.commandServers=window.__savedTerminalServers;renderTerminalView()`, nil)); err != nil {
		t.Fatal(err)
	}

	// The command tab lists the command sets and sends one to the shell.
	if err := chromedp.Run(ctx,
		chromedp.Click(`[data-term-pane="commands"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`[data-send-flow="flow-0"]`, chromedp.ByQuery),
		chromedp.Click(`[data-send-flow="flow-0"]`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("send a command set: %v", err)
	}
	wait(`document.querySelector('.term-session.active .xterm-rows').innerText.includes('df -h')`, "the command set to reach the shell")

	// Closing the tab ends the session.
	if err := chromedp.Run(ctx, chromedp.Click(`.term-tab-close`, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	wait(`document.querySelectorAll('.term-tab').length === 0 && !document.querySelector('#term-empty').classList.contains('hidden')`, "the session to close")
}

func filepathJoinTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
