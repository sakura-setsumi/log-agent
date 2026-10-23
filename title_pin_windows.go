//go:build windows

package main

import (
	"fmt"
	"math"
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
	pinCreatePen       = pinGDI.NewProc("CreatePen")
	pinSelectObject    = pinGDI.NewProc("SelectObject")
	pinDeleteObject    = pinGDI.NewProc("DeleteObject")
	pinMoveTo          = pinGDI.NewProc("MoveToEx")
	pinLineTo          = pinGDI.NewProc("LineTo")
	pinGetCaptionColor = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmGetWindowAttribute")
	pinOwnerCallback   uintptr
	pinButtonCallback  uintptr
	captionPins        = map[uintptr]*captionPin{}
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
	captionPins[owner], captionPins[p.button] = p, p
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
	delete(captionPins, p.owner)
	delete(captionPins, p.button)
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
		procShowWindow.Call(p.button, 0)
		return
	}
	dpi, _, _ := pinGetDPI.Call(p.owner)
	width := int32(40 * dpi / 96)
	if width < 24 {
		width = 40
	}
	pinSetWindowPos.Call(p.button, 0, uintptr(minimize.Left-width), uintptr(minimize.Top),
		uintptr(width), uintptr(minimize.Bottom-minimize.Top), 0x0054) // NOZORDER | NOACTIVATE | SHOWWINDOW
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
	// The glyph angle indicates state; the background only changes on hover.
	pinInvalidate.Call(p.button, 0, 0)
}

func captionPinOwnerProc(hwnd, msg, wp, lp, id, data uintptr) uintptr {
	p := captionPins[hwnd]
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
	p := captionPins[hwnd]
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
	background, foreground := uint32(0xF3F3F3), uint32(0x444444)
	var captionColor uint32
	result, _, _ := pinGetCaptionColor.Call(p.owner, 35, uintptr(unsafe.Pointer(&captionColor)), 4)
	if result == 0 && captionColor < 0x1000000 {
		background = captionColor
	}
	var dark int32
	pinGetCaptionColor.Call(p.owner, 20, uintptr(unsafe.Pointer(&dark)), 4)
	if dark != 0 {
		background, foreground = 0x202020, 0xEEEEEE
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
		foreground = 0x1677C4 // #c47716, COLORREF uses BGR byte order.
		if dark != 0 {
			foreground = 0x37A6F6
		} // #f6a637
	}
	brush, _, _ := pinCreateBrush.Call(uintptr(background))
	pinFillRect.Call(dc, uintptr(unsafe.Pointer(&rect)), brush)
	pinDeleteObject.Call(brush)
	scale := float64(rect.Bottom) / 30
	penWidth := uintptr(math.Max(1, math.Round(scale)))
	pen, _, _ := pinCreatePen.Call(0, penWidth, uintptr(foreground))
	old, _, _ := pinSelectObject.Call(dc, pen)
	// A small outlined pushpin. Coordinates are relative to the button centre.
	points := [][2]float64{{-4, -6}, {4, -6}, {4, -4}, {2, -4}, {2, 0}, {5, 3}, {-5, 3}, {-2, 0}, {-2, -4}, {-4, -4}, {-4, -6}}
	angle := math.Pi / 6
	if style&8 != 0 {
		angle = 0
	}
	coord := func(x, y float64) (uintptr, uintptr) {
		rx, ry := x*math.Cos(angle)-y*math.Sin(angle), x*math.Sin(angle)+y*math.Cos(angle)
		return uintptr(int32(math.Round(float64(rect.Right)/2 + rx*scale))), uintptr(int32(math.Round(float64(rect.Bottom)/2 + ry*scale)))
	}
	for i, point := range points {
		x, y := coord(point[0], point[1])
		if i == 0 {
			pinMoveTo.Call(dc, x, y, 0)
		} else {
			pinLineTo.Call(dc, x, y)
		}
	}
	x, y := coord(0, 3)
	pinMoveTo.Call(dc, x, y, 0)
	x, y = coord(0, 8)
	pinLineTo.Call(dc, x, y)
	pinSelectObject.Call(dc, old)
	pinDeleteObject.Call(pen)
}
