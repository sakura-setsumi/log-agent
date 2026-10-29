//go:build windows

package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pinComctl          = windows.NewLazySystemDLL("comctl32.dll")
	pinGDI             = windows.NewLazySystemDLL("gdi32.dll")
	pinSetSubclass     = pinComctl.NewProc("SetWindowSubclass")
	pinRemoveSubclass  = pinComctl.NewProc("RemoveWindowSubclass")
	pinDefSubclass     = pinComctl.NewProc("DefSubclassProc")
	pinCreateWindow    = user32.NewProc("CreateWindowExW")
	pinDestroyWindow   = user32.NewProc("DestroyWindow")
	pinSetWindowPos    = user32.NewProc("SetWindowPos")
	pinIsVisible       = user32.NewProc("IsWindowVisible")
	pinGetWindowLong   = user32.NewProc("GetWindowLongW")
	pinGetClientRect   = user32.NewProc("GetClientRect")
	pinGetDPI          = user32.NewProc("GetDpiForWindow")
	pinSetText         = user32.NewProc("SetWindowTextW")
	pinInvalidate      = user32.NewProc("InvalidateRect")
	pinBeginPaint      = user32.NewProc("BeginPaint")
	pinEndPaint        = user32.NewProc("EndPaint")
	pinFillRect        = user32.NewProc("FillRect")
	pinCreateBrush     = pinGDI.NewProc("CreateSolidBrush")
	pinCreateFont      = pinGDI.NewProc("CreateFontW")
	pinSetTextColor    = pinGDI.NewProc("SetTextColor")
	pinSetBkMode       = pinGDI.NewProc("SetBkMode")
	pinDrawText        = user32.NewProc("DrawTextW")
	pinSelectObject    = pinGDI.NewProc("SelectObject")
	pinDeleteObject    = pinGDI.NewProc("DeleteObject")
	pinGetCaptionColor = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmGetWindowAttribute")
	pinOwnerCallback   uintptr
	pinButtonCallback  uintptr
	captionPins        sync.Map
)

func init() {
	pinOwnerCallback = windows.NewCallback(captionPinOwnerProc)
	pinButtonCallback = windows.NewCallback(captionPinButtonProc)
}

type pinRect struct{ Left, Top, Right, Bottom int32 }
type pinTitleInfo struct {
	Size  uint32
	Title pinRect
	State [6]uint32
	Rects [6]pinRect
}
type pinPaintInfo struct {
	DC                 uintptr
	Erase              int32
	Rect               pinRect
	Restore, IncUpdate int32
	Reserved           [32]byte
}
type pinToolInfo struct {
	Size, Flags                     uint32
	Window, ID                      uintptr
	Rect                            pinRect
	Instance, Text, Param, Reserved uintptr
}

type pinMouseTracking struct {
	Size, Flags uint32
	Window      uintptr
	HoverTime   uint32
}

type captionPin struct {
	owner, button, tooltip uintptr
	label                  *uint16
	closed                 bool
	pressed                bool
	hovered                bool
}

// A non-activating owned button can occupy the native caption without replacing
// Windows' frame or taking space from the WebView client area. Ownership keeps it
// above this app only; the owner subclass tracks movement, DPI and visibility.
func installCaptionPin(owner uintptr) (*captionPin, error) {
	p := &captionPin{owner: owner}
	module, _, _ := procGetModuleHandle.Call(0)
	p.label = windows.StringToUTF16Ptr("窗口置顶")
	p.button, _, _ = pinCreateWindow.Call(0x08000080, // WS_EX_NOACTIVATE | WS_EX_TOOLWINDOW
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("BUTTON"))), uintptr(unsafe.Pointer(p.label)),
		0x8000000B, 0, 0, 40, 30, owner, 0, module, 0) // WS_POPUP | BS_OWNERDRAW
	if p.button == 0 {
		return nil, fmt.Errorf("create title-bar pin button")
	}
	captionPins.Store(owner, p)
	captionPins.Store(p.button, p)
	ok, _, _ := pinSetSubclass.Call(owner, pinOwnerCallback, 1, 0)
	if ok == 0 {
		p.close()
		return nil, fmt.Errorf("subclass title-bar pin owner")
	}
	ok, _, _ = pinSetSubclass.Call(p.button, pinButtonCallback, 1, 0)
	if ok == 0 {
		p.close()
		return nil, fmt.Errorf("subclass title-bar pin button")
	}
	p.tooltip, _, _ = pinCreateWindow.Call(0x08000080,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("tooltips_class32"))), 0,
		0x80000003, 0, 0, 0, 0, owner, 0, module, 0)
	if p.tooltip != 0 {
		tool := p.toolInfo()
		procSendMessage.Call(p.tooltip, 0x0432, 0, uintptr(unsafe.Pointer(&tool))) // TTM_ADDTOOLW
	}
	p.position()
	return p, nil
}

func (p *captionPin) toolInfo() pinToolInfo {
	tool := pinToolInfo{Flags: 0x11, Window: p.button, ID: p.button, Text: uintptr(unsafe.Pointer(p.label))}
	tool.Size = uint32(unsafe.Sizeof(tool))
	return tool
}

func (p *captionPin) close() {
	if p.closed {
		return
	}
	p.closed = true
	pinRemoveSubclass.Call(p.owner, pinOwnerCallback, 1)
	pinRemoveSubclass.Call(p.button, pinButtonCallback, 1)
	captionPins.Delete(p.owner)
	captionPins.Delete(p.button)
	if p.tooltip != 0 {
		pinDestroyWindow.Call(p.tooltip)
	}
	pinDestroyWindow.Call(p.button)
}

func (p *captionPin) position() {
	if p.closed {
		return
	}
	visible, _, _ := pinIsVisible.Call(p.owner)
	minimized, _, _ := procIsIconic.Call(p.owner)
	if visible == 0 || minimized != 0 {
		procShowWindow.Call(p.button, 0)
		return
	}
	info := pinTitleInfo{}
	info.Size = uint32(unsafe.Sizeof(info))
	procSendMessage.Call(p.owner, 0x033F, 0, uintptr(unsafe.Pointer(&info))) // WM_GETTITLEBARINFOEX
	minimize := info.Rects[2]
	if minimize.Right <= minimize.Left || minimize.Bottom <= minimize.Top {
		// Caption bounds can be transiently unavailable during a frame change.
		// Keep the last valid position instead of hiding the button.
		return
	}
	dpi, _, _ := pinGetDPI.Call(p.owner)
	width := int32(40 * dpi / 96)
	if width < 24 {
		width = 40
	}
	pinSetWindowPos.Call(p.button, 0, uintptr(minimize.Left-width), uintptr(minimize.Top),
		uintptr(width), uintptr(minimize.Bottom-minimize.Top), 0x0250) // HWND_TOP; NOOWNERZORDER | NOACTIVATE | SHOWWINDOW
}

func (p *captionPin) toggle() {
	// Read the real window state so external changes cannot desynchronise it.
	style, _, _ := pinGetWindowLong.Call(p.owner, ^uintptr(19)) // GWL_EXSTYLE = -20
	after := ^uintptr(0)                                        // HWND_TOPMOST
	label := "取消窗口置顶"
	if style&0x8 != 0 {
		after = ^uintptr(1)
		label = "窗口置顶"
	} // HWND_NOTOPMOST
	ok, _, _ := pinSetWindowPos.Call(p.owner, after, 0, 0, 0, 0, 0x0013) // NOMOVE | NOSIZE | NOACTIVATE
	if ok == 0 {
		return
	}
	p.label = windows.StringToUTF16Ptr(label)
	pinSetText.Call(p.button, uintptr(unsafe.Pointer(p.label)))
	if p.tooltip != 0 {
		tool := p.toolInfo()
		procSendMessage.Call(p.tooltip, 0x0439, 0, uintptr(unsafe.Pointer(&tool))) // TTM_UPDATETIPTEXTW
	}
	// Restore placement and stacking after Windows changes the owner topmost band.
	p.position()
	user32.NewProc("RedrawWindow").Call(p.button, 0, 0, 0x0101) // INVALIDATE | UPDATENOW
}

func captionPinOwnerProc(hwnd, msg, wp, lp, id, data uintptr) uintptr {
	value, _ := captionPins.Load(hwnd)
	p, _ := value.(*captionPin)
	if p != nil && msg == 0x0082 {
		p.close()
	} // WM_NCDESTROY
	result, _, _ := pinDefSubclass.Call(hwnd, msg, wp, lp)
	if p != nil && !p.closed {
		switch msg {
		case 0x0047, 0x0005, 0x0018, 0x02E0, 0x007D: // position, size, show, DPI, style
			p.position()
			if msg == 0x007D {
				pinInvalidate.Call(p.button, 0, 0)
			}
		case 0x001C, 0x031A: // activation and theme
			pinInvalidate.Call(p.button, 0, 0)
		}
	}
	return result
}

func captionPinButtonProc(hwnd, msg, wp, lp, id, data uintptr) uintptr {
	value, _ := captionPins.Load(hwnd)
	p, _ := value.(*captionPin)
	if p != nil {
		switch msg {
		case 0x0318: // WM_PRINTCLIENT: support native previews and paint verification.
			p.draw(wp)
			return 0
		case 0x000F:
			p.paint()
			return 0 // WM_PAINT
		case 0x0014:
			return 1 // WM_ERASEBKGND
		case 0x0021:
			return 3 // WM_MOUSEACTIVATE: MA_NOACTIVATE
		case 0x00F5:
			p.toggle()
			return 0 // BM_CLICK (also available to accessibility clients)
		case 0x0200: // WM_MOUSEMOVE
			if !p.hovered {
				p.hovered = true
				tracking := pinMouseTracking{Flags: 2, Window: hwnd} // TME_LEAVE
				tracking.Size = uint32(unsafe.Sizeof(tracking))
				user32.NewProc("TrackMouseEvent").Call(uintptr(unsafe.Pointer(&tracking)))
				pinInvalidate.Call(hwnd, 0, 0)
			}
		case 0x02A3: // WM_MOUSELEAVE
			p.hovered = false
			pinInvalidate.Call(hwnd, 0, 0)
		case 0x0201, 0x0203: // WM_LBUTTONDOWN / WM_LBUTTONDBLCLK
			p.pressed = true
			user32.NewProc("SetCapture").Call(hwnd)
			return 0
		case 0x0202: // WM_LBUTTONUP: drag-out cancels instead of toggling.
			pressed := p.pressed
			p.pressed = false
			user32.NewProc("ReleaseCapture").Call()
			var rect pinRect
			pinGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
			x, y := int32(int16(lp&0xffff)), int32(int16((lp>>16)&0xffff))
			if pressed && x >= 0 && y >= 0 && x < rect.Right && y < rect.Bottom {
				p.toggle()
			}
			return 0
		case 0x0215, 0x001F: // WM_CAPTURECHANGED / WM_CANCELMODE
			p.pressed = false

		}
	}
	result, _, _ := pinDefSubclass.Call(hwnd, msg, wp, lp)
	return result
}

func (p *captionPin) paint() {
	var paint pinPaintInfo
	dc, _, _ := pinBeginPaint.Call(p.button, uintptr(unsafe.Pointer(&paint)))
	if dc == 0 {
		return
	}
	defer pinEndPaint.Call(p.button, uintptr(unsafe.Pointer(&paint)))
	p.draw(dc)
}

func (p *captionPin) draw(dc uintptr) {
	var rect pinRect
	pinGetClientRect.Call(p.button, uintptr(unsafe.Pointer(&rect)))
	background, foreground := uint32(0xF3F3F3), uint32(0x000000)
	var captionColor uint32
	result, _, _ := pinGetCaptionColor.Call(p.owner, 35, uintptr(unsafe.Pointer(&captionColor)), 4)
	if result == 0 && captionColor < 0x1000000 {
		background = captionColor
	}
	var dark int32
	pinGetCaptionColor.Call(p.owner, 20, uintptr(unsafe.Pointer(&dark)), 4)
	if dark != 0 {
		background, foreground = 0x202020, 0xFFFFFF
	}
	if p.hovered {
		if dark != 0 {
			background = 0x383838
		} else {
			background = 0xE5E5E5
		}
	}
	style, _, _ := pinGetWindowLong.Call(p.owner, ^uintptr(19))
	if style&8 != 0 {
		foreground = 0x1677C4 // Selected: original orange, in COLORREF (BGR) order.
		if dark != 0 {
			foreground = 0x37A6F6
		}
	}
	brush, _, _ := pinCreateBrush.Call(uintptr(background))
	pinFillRect.Call(dc, uintptr(unsafe.Pointer(&rect)), brush)
	pinDeleteObject.Call(brush)
	// Use Microsoft's stock Pin/Pinned glyphs, rendered by the system icon font.
	// Windows 10 provides the same glyphs through Segoe MDL2 Assets.
	scale := float64(rect.Bottom) / 30
	height := int32(math.Max(1, math.Round(16*0.9*scale)))
	font, _, _ := pinCreateFont.Call(uintptr(-height), 0, 0, 0, 400, 0, 0, 0,
		1, 0, 0, 3, 0, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(captionPinIconFont)))) // NONANTIALIASED_QUALITY: match the native caption strokes
	if font == 0 {
		return
	}
	oldFont, _, _ := pinSelectObject.Call(dc, font)
	defer func() {
		pinSelectObject.Call(dc, oldFont)
		pinDeleteObject.Call(font)
	}()
	oldColor, _, _ := pinSetTextColor.Call(dc, uintptr(foreground))
	defer pinSetTextColor.Call(dc, oldColor)
	oldMode, _, _ := pinSetBkMode.Call(dc, 1) // TRANSPARENT
	defer pinSetBkMode.Call(dc, oldMode)
	glyph := uint16(0xE718) // Pin
	if style&8 != 0 {
		glyph = 0xE840 // Pinned
	}
	pinDrawText.Call(dc, uintptr(unsafe.Pointer(&glyph)), 1, uintptr(unsafe.Pointer(&rect)),
		0x0001|0x0004|0x0020|0x0800) // CENTER | VCENTER | SINGLELINE | NOPREFIX
}

// Prefer Windows 11's Fluent library, with the Windows 10 icon library as fallback.
var captionPinIconFont = func() string {
	if _, err := os.Stat(filepath.Join(os.Getenv("SystemRoot"), "Fonts", "SegoeIcons.ttf")); err == nil {
		return "Segoe Fluent Icons"
	}
	return "Segoe MDL2 Assets"
}()
