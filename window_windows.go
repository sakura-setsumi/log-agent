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
	procExtractIcon         = shell32.NewProc("ExtractIconW")
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
	debug := strings.TrimSpace(os.Getenv("LOG_AGENT_DEBUG")) == "1"
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     debug,
		DataPath:  dataPath,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  windowTitle,
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
	w.SetSize(1440, 900, webview2.HintNone)
	setWindowIcon(uintptr(w.Window()))

	if err := serveFrontend(w, handler); err != nil {
		return err
	}

	b := newBridge(handler, func(script string) {
		w.Dispatch(func() { w.Eval(script) })
	})
	bindings := map[string]interface{}{
		"__lbBody":         func(id, chunk string) { b.body(id, chunk) },
		"__lbRequest":      func(id, method, target string, headers map[string]string) { b.request(id, method, target, headers) },
		"__lbCancel":       func(id string) { b.cancel(id) },
		"__lbOpenExternal": openExternal,
		"__lbQuit":         func() { w.Destroy() },
	}
	for name, fn := range bindings {
		if err := w.Bind(name, fn); err != nil {
			return fmt.Errorf("bind %s: %w", name, err)
		}
	}
	w.Init(bridgeScript)

	w.Navigate(bridgeOrigin + "/")
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
func serveFrontend(w webview2.WebView, handler http.Handler) error {
	field := reflect.ValueOf(w).Elem().FieldByName("browser")
	if !field.IsValid() {
		return errors.New("unsupported go-webview2 version: no browser field")
	}
	browser := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface()
	chromium, ok := browser.(*edge.Chromium)
	if !ok {
		return fmt.Errorf("unsupported go-webview2 version: browser is %T", browser)
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
		return
	}
	if minimized, _, _ := procIsIconic.Call(hwnd); minimized != 0 {
		procShowWindow.Call(hwnd, uintptr(windows.SW_RESTORE))
	}
	procSetForegroundWindow.Call(hwnd)
}

// setWindowIcon uses the icon compiled into the executable's resources.
func setWindowIcon(hwnd uintptr) {
	executable, err := os.Executable()
	if err != nil {
		return
	}
	const wmSetIcon, iconSmall, iconBig = 0x0080, 0, 1
	icon, _, _ := procExtractIcon.Call(0, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(executable))), 0)
	if icon <= 1 { // 0: no icon, 1: not an executable
		return
	}
	procSendMessage.Call(hwnd, wmSetIcon, iconBig, icon)
	procSendMessage.Call(hwnd, wmSetIcon, iconSmall, icon)
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
