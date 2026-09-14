package main

import (
	"context"
	"encoding/base64"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		chromedp.Click(`#assistant-session-toggle`, chromedp.ByQuery),
		chromedp.WaitVisible(`#assistant-session-list .assistant-session-item:first-child [data-delete-assistant-session]`, chromedp.ByQuery),
		chromedp.Click(`#assistant-session-list .assistant-session-item:first-child [data-delete-assistant-session]`, chromedp.ByQuery),
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
			const markdown = '# 安全渲染\n[安全链接](https://example.com/docs)\n[危险链接](javascript:alert(1))\n<img src=x onerror="window.__assistantMarkdownExecuted=true">\n<script>window.__assistantMarkdownExecuted=true</script>\n' + fence + '\nconst ok = true;\n' + fence + '\n| 名称 | 结果 |\n| --- | --- |\n| 安全 | **通过** |';
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
			};
		})()`, &markdownSafety),
	); err != nil {
		t.Fatalf("verify markdown rendering safety: %v", err)
	}
	if !markdownSafety.HasCodeBlock || !markdownSafety.HasTable || !markdownSafety.SafeLink || !markdownSafety.SameBackground || markdownSafety.HasRawImage || markdownSafety.HasScriptTag || markdownSafety.UnsafeLink || markdownSafety.XSSExecuted {
		t.Fatalf("unsafe markdown rendering state: %+v", markdownSafety)
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
