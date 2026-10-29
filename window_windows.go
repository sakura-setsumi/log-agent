//go:build windows

package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

//go:embed bridge.js
var bridgeScript string

const (
	windowTitle     = "栈桥"
	windowClass     = "webview" // the class go-webview2 registers
	singleInstance  = `Local\LogAgentDesktop-`
	maxLogFileBytes = 5 << 20
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	shell32                 = windows.NewLazySystemDLL("shell32.dll")
	procFindWindow          = user32.NewProc("FindWindowW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procIsIconic            = user32.NewProc("IsIconic")
	procMessageBox          = user32.NewProc("MessageBoxW")
	procSendMessage         = user32.NewProc("SendMessageW")
	procLoadImage           = user32.NewProc("LoadImageW")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procGetModuleHandle     = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW")
)

func init() {
	// The window and its message loop must stay on the thread that created them.
	runtime.LockOSThread()
}

// runDesktop opens the dashboard in a native window and blocks until it is
// closed. Static files are served from the embedded frontend by intercepting
// the webview's requests, and /api/ calls go through the bridge, so the process
// never opens a port.
func runDesktop(handler http.Handler) error {
	closeLog := redirectLog()
	defer closeLog()

	mutex, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(instanceName()))
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return fmt.Errorf("single-instance lock: %w", err)
	}
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		windows.CloseHandle(mutex)
		focusRunningWindow()
		return nil
	}
	defer windows.CloseHandle(mutex)

	dataPath := filepath.Join(configDir(), "webview2")
	if err := os.MkdirAll(dataPath, 0o755); err != nil {
		return fmt.Errorf("webview data folder: %w", err)
	}
	manager := &desktopWindows{handler: handler, dataPath: dataPath, done: make(chan struct{}), active: 1}
	err = manager.run("", 0, 0, nil)
	manager.release()
	<-manager.done
	return err
}

// Each WebView owns an STA thread and message loop. Closing one window only
// quits that thread; keep the shared backend alive until the last window closes.
type desktopWindows struct {
	handler  http.Handler
	dataPath string
	mu       sync.Mutex
	active   int
	closed   bool
	done     chan struct{}
	handles  map[uintptr]string
}

var detachedViewTitles = map[string]string{
	"overview": "实时日志流", "rules": "加工规则", "connections": "连接管理",
	"commands": "指令集组", "terminal": "服务器", "database": "数据库", "redis": "Redis",
}

func (m *desktopWindows) release() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active--
	if m.active == 0 {
		m.closed = true
		close(m.done)
	}
}

func (m *desktopWindows) open(view string, x, y int) error {
	ready, err := m.start(view, x, y)
	if err != nil {
		return err
	}
	return <-ready
}

func (m *desktopWindows) start(view string, x, y int) (<-chan error, error) {
	if _, ok := detachedViewTitles[view]; !ok {
		return nil, fmt.Errorf("unknown view: %s", view)
	}
	if x < -100000 || x > 100000 || y < -100000 || y > 100000 {
		return nil, errors.New("invalid window position")
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, errors.New("application has closed")
	}
	m.active++
	m.mu.Unlock()
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer m.release()
		if err := m.run(view, x, y, ready); err != nil {
			ready <- err
		}
	}()
	return ready, nil
}

func (m *desktopWindows) run(view string, x, y int, ready chan<- error) error {
	// The library initializes COM only on its package-init thread.
	ole32 := windows.NewLazySystemDLL("ole32.dll")
	hr, _, _ := ole32.NewProc("CoInitializeEx").Call(0, 2)
	if int32(hr) < 0 {
		return fmt.Errorf("initialize window COM: %08x", hr)
	}
	defer ole32.NewProc("CoUninitialize").Call()
	title := windowTitle
	if view != "" {
		title += " · " + detachedViewTitles[view]
	}
	debug := strings.TrimSpace(os.Getenv("LOG_AGENT_DEBUG")) == "1"
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     debug,
		DataPath:  m.dataPath,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  title,
			Width:  1440,
			Height: 900,
			Center: true,
		},
	})
	if w == nil {
		message := "无法创建窗口：未检测到 Microsoft Edge WebView2 运行时。\n请安装后重试：https://developer.microsoft.com/microsoft-edge/webview2/"
		showError(message)
		return errors.New("WebView2 runtime is not available")
	}
	defer w.Destroy()
	defer closeDesktopBrowser(w)
	hwnd := uintptr(w.Window())
	m.mu.Lock()
	if m.handles == nil {
		m.handles = make(map[uintptr]string)
	}
	m.handles[hwnd] = view
	m.mu.Unlock()
	defer func() { m.mu.Lock(); delete(m.handles, hwnd); m.mu.Unlock() }()
	w.SetSize(1440, 900, webview2.HintNone)
	if view != "" {
		pinSetWindowPos.Call(uintptr(w.Window()), 0, uintptr(x-40), uintptr(y-16), 0, 0, 0x0015)
	}
	setWindowIcon(uintptr(w.Window()))
	pin, err := installCaptionPin(uintptr(w.Window()))
	if err != nil {
		return err
	}
	defer pin.close()

	if err := serveFrontend(w, m.handler); err != nil {
		return err
	}

	var alive atomic.Bool
	alive.Store(true)
	emit := func(script string) {
		if alive.Load() {
			w.Dispatch(func() {
				if alive.Load() {
					w.Eval(script)
				}
			})
		}
	}
	b := newBridge(m.handler, emit)
	defer func() { alive.Store(false); b.close() }()
	bindings := map[string]interface{}{
		"__lbBody":         func(id, chunk string) { b.body(id, chunk) },
		"__lbRequest":      func(id, method, target string, headers map[string]string) { b.request(id, method, target, headers) },
		"__lbCancel":       func(id string) { b.cancel(id) },
		"__lbOpenExternal": openExternal,
		"__lbQuit":         func() { w.Destroy() },
		// WebMessageReceived runs on the caller's STA. Never wait for another
		// WebView to initialize inside that callback: the shared runtime needs
		// the original STA to keep pumping messages during window creation.
		"__lbOpenViewRequest": func(token, view string, x, y int) error {
			ready, err := m.start(view, x, y)
			if err != nil {
				return err
			}
			go func() {
				message := ""
				if err := <-ready; err != nil {
					message = err.Error()
				}
				emit(fmt.Sprintf("window.__lbViewResult(%s,%s)", jsonString(token), jsonString(message)))
			}()
			return nil
		},
	}
	for name, fn := range bindings {
		if err := w.Bind(name, fn); err != nil {
			return fmt.Errorf("bind %s: %w", name, err)
		}
	}
	w.Init(bridgeScript)

	target := bridgeOrigin + "/"
	if view != "" {
		target += "?detached=" + view
	}
	w.Navigate(target)
	if ready != nil {
		ready <- nil
	}
	w.Run()
	return nil
}

// instanceName keys the single-instance lock to the data directory, so two
// installs (or an isolated test run) with different data do not block each other.
func instanceName() string {
	sum := sha256.Sum256([]byte(strings.ToLower(configDir())))
	return singleInstance + hex.EncodeToString(sum[:8])
}

// serveFrontend answers requests for the virtual origin from the embedded
// files. go-webview2 does not expose the underlying browser, so it is read from
// the unexported field; the library version is pinned in go.mod.
func desktopBrowser(w webview2.WebView) (*edge.Chromium, error) {
	field := reflect.ValueOf(w).Elem().FieldByName("browser")
	if !field.IsValid() {
		return nil, errors.New("unsupported go-webview2 version: no browser field")
	}
	browser := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface()
	chromium, ok := browser.(*edge.Chromium)
	if !ok {
		return nil, fmt.Errorf("unsupported go-webview2 version: browser is %T", browser)
	}
	return chromium, nil
}

// The pinned wrapper does not close its controller or release its COM objects
// when Destroy posts WM_CLOSE. Release them on the owning STA after Run returns.
func closeDesktopBrowser(w webview2.WebView) {
	chromium, err := desktopBrowser(w)
	if err != nil {
		return
	}
	call := func(object reflect.Value, method string) {
		if object.IsNil() {
			return
		}
		fn := object.Elem().FieldByName("vtbl").Elem().FieldByName(method).Uint()
		syscall.SyscallN(uintptr(fn), object.Pointer())
	}
	value := reflect.ValueOf(chromium).Elem()
	controller, view, environment := value.FieldByName("controller"), value.FieldByName("webview"), value.FieldByName("environment")
	call(controller, "Close")
	// GetCoreWebView2 and the wrapper's extra AddRef each hold a reference.
	call(view, "Release")
	call(view, "Release")
	call(controller, "Release")
	call(environment, "Release")
}

func serveFrontend(w webview2.WebView, handler http.Handler) error {
	chromium, err := desktopBrowser(w)
	if err != nil {
		return err
	}

	chromium.WebResourceRequestedCallback = func(request *edge.ICoreWebView2WebResourceRequest, args *edge.ICoreWebView2WebResourceRequestedEventArgs) {
		uri, err := request.GetUri()
		if err != nil {
			return
		}
		target := strings.TrimPrefix(uri, bridgeOrigin)
		if target == "" {
			target = "/"
		}
		status, header, body := serveStatic(handler, target)
		var headers strings.Builder
		for name, values := range header {
			for _, value := range values {
				fmt.Fprintf(&headers, "%s: %s\r\n", name, value)
			}
		}
		response, err := chromium.Environment().CreateWebResourceResponse(body, status, http.StatusText(status), headers.String())
		if err != nil {
			log.Printf("static response for %s: %v", target, err)
			return
		}
		if err := args.PutResponse(response); err != nil {
			log.Printf("static response for %s: %v", target, err)
		}
	}
	chromium.AddWebResourceRequestedFilter(bridgeOrigin+"/*", edge.COREWEBVIEW2_WEB_RESOURCE_CONTEXT_ALL)

	if settings, err := chromium.GetSettings(); err == nil {
		_ = settings.PutIsZoomControlEnabled(false)
		_ = settings.PutIsStatusBarEnabled(false)
	}
	return nil
}

// openExternal hands a link to the system browser. Only web links are accepted.
func openExternal(target string) {
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		return
	}
	if err := windows.ShellExecute(0, windows.StringToUTF16Ptr("open"), windows.StringToUTF16Ptr(target), nil, nil, windows.SW_SHOWNORMAL); err != nil {
		log.Printf("could not open %s: %v", target, err)
	}
}

// focusRunningWindow brings the already-running instance's window forward.
func focusRunningWindow() {
	hwnd, _, _ := procFindWindow.Call(
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(windowClass))),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(windowTitle))),
	)
	if hwnd == 0 {
		// The main window may have closed while detached pages are still open.
		for _, label := range detachedViewTitles {
			hwnd, _, _ = procFindWindow.Call(
				uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(windowClass))),
				uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(windowTitle+" · "+label))),
			)
			if hwnd != 0 {
				break
			}
		}
		if hwnd == 0 {
			return
		}
	}
	if minimized, _, _ := procIsIconic.Call(hwnd); minimized != 0 {
		procShowWindow.Call(hwnd, uintptr(windows.SW_RESTORE))
	}
	procSetForegroundWindow.Call(hwnd)
}

// Load each window icon directly from resource ID 1 (go-winres simply),
// at its system size. Shared resource handles live for the application lifetime.
func setWindowIcon(hwnd uintptr) {
	module, _, _ := procGetModuleHandle.Call(0)
	if module == 0 {
		return
	}
	const wmSetIcon, imageIcon, lrShared = 0x0080, 1, 0x8000
	for _, spec := range []struct{ kind, widthMetric, heightMetric uintptr }{
		{1, 11, 12}, // ICON_BIG, SM_CXICON, SM_CYICON
		{0, 49, 50}, // ICON_SMALL, SM_CXSMICON, SM_CYSMICON
	} {
		width, _, _ := procGetSystemMetrics.Call(spec.widthMetric)
		height, _, _ := procGetSystemMetrics.Call(spec.heightMetric)
		icon, _, _ := procLoadImage.Call(module, 1, imageIcon, width, height, lrShared)
		if icon != 0 {
			procSendMessage.Call(hwnd, wmSetIcon, spec.kind, icon)
		}
	}
}

func showError(message string) {
	procMessageBox.Call(0,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(message))),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(windowTitle))),
		0x10, // MB_ICONERROR
	)
}

// redirectLog sends the log to a file beside the configuration: a GUI build
// has no console to read it from.
func redirectLog() func() {
	path := filepath.Join(configDir(), "log-agent.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return func() {}
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogFileBytes {
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	file, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return func() {}
	}
	log.SetOutput(file)
	return func() { _ = file.Close() }
}
