//go:build windows

package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestCaptionPinNativeWindow(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	module, _, _ := procGetModuleHandle.Call(0)
	owner, _, err := pinCreateWindow.Call(0,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("STATIC"))),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("Pin test"))),
		0x00CF0000, // WS_OVERLAPPEDWINDOW; keep this test window offscreen
		uintptr(uint32(0xFFFFB1E0)), uintptr(uint32(0xFFFFB1E0)), 640, 480, 0, 0, module, 0)
	if owner == 0 {
		t.Fatalf("create test window: %v", err)
	}
	defer pinDestroyWindow.Call(owner)
	p, err := installCaptionPin(owner)
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()
	if visible, _, _ := pinIsVisible.Call(p.button); visible != 0 {
		t.Fatal("pin is visible while owner is hidden")
	}
	getStyle := func() uintptr { style, _, _ := pinGetWindowLong.Call(owner, ^uintptr(19)); return style }
	getText := func() string {
		var buf [80]uint16
		user32.NewProc("GetWindowTextW").Call(p.button, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		return windows.UTF16ToString(buf[:])
	}
	if getStyle()&8 != 0 {
		t.Fatal("window starts topmost")
	}
	procSendMessage.Call(p.button, 0x00F5, 0, 0)
	if getStyle()&8 == 0 || getText() != "取消窗口置顶" {
		t.Fatal("first pin click did not make window topmost")
	}
	procSendMessage.Call(p.button, 0x00F5, 0, 0)
	if getStyle()&8 != 0 || getText() != "窗口置顶" {
		t.Fatal("second pin click did not clear topmost")
	}
	// The visible window is offscreen. Caption placement should track the real
	// minimise-button bounds rather than hardcoded screen coordinates.
	procShowWindow.Call(owner, 4) // SW_SHOWNOACTIVATE
	var info pinTitleInfo
	info.Size = uint32(unsafe.Sizeof(info))
	procSendMessage.Call(owner, 0x033F, 0, uintptr(unsafe.Pointer(&info)))
	var rect pinRect
	getRect := user32.NewProc("GetWindowRect")
	getRect.Call(p.button, uintptr(unsafe.Pointer(&rect)))
	if visible, _, _ := pinIsVisible.Call(p.button); visible == 0 {
		t.Fatal("pin did not follow visible owner")
	}
	if rect.Right != info.Rects[2].Left || rect.Top != info.Rects[2].Top || rect.Bottom != info.Rects[2].Bottom {
		t.Fatalf("pin overlaps caption buttons: pin=%+v minimise=%+v", rect, info.Rects[2])
	}
	// Repeated real click messages must leave the caption button visible above
	// its owner in both the normal and topmost bands.
	for i := 0; i < 20; i++ {
		procSendMessage.Call(p.button, 0x0201, 1, 10|(10<<16))
		procSendMessage.Call(p.button, 0x0202, 0, 10|(10<<16))
		if visible, _, _ := pinIsVisible.Call(p.button); visible == 0 {
			t.Fatalf("pin disappeared after click %d", i+1)
		}
		found := false
		for window := owner; window != 0; {
			window, _, _ = user32.NewProc("GetWindow").Call(window, 3) // GW_HWNDPREV
			if window == p.button {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("pin fell behind owner after click %d", i+1)
		}
	}
	before := captureCaptionPin(t, p)
	ink := 0
	background := before.RGBAAt(0, 0)
	for y := 0; y < before.Bounds().Dy(); y++ {
		for x := 0; x < before.Bounds().Dx(); x++ {
			pixel := before.RGBAAt(x, y)
			if pixel != background {
				if pixel != (color.RGBA{A: 255}) {
					t.Fatalf("unselected pin has faded stroke at (%d,%d): %v", x, y, pixel)
				}
				ink++
			}
		}
	}
	if ink == 0 {
		t.Fatal("unselected pin has no visible strokes")
	}
	procSendMessage.Call(p.button, 0x00F5, 0, 0)
	after := captureCaptionPin(t, p)
	if slices.Equal(before.Pix, after.Pix) {
		t.Fatal("pin glyph did not change while topmost")
	}
	if before.RGBAAt(0, 0) != after.RGBAAt(0, 0) {
		t.Fatal("topmost state highlights the background")
	}
	procSendMessage.Call(p.button, 0x0200, 0, 10|(10<<16))
	hover := captureCaptionPin(t, p)
	if hover.RGBAAt(0, 0) != (color.RGBA{R: 229, G: 229, B: 229, A: 255}) {
		t.Fatal("hover did not show a light-grey background")
	}
	procSendMessage.Call(p.button, 0x02A3, 0, 0)
	if !slices.Equal(after.Pix, captureCaptionPin(t, p).Pix) {
		t.Fatal("hover background remained after mouse leave")
	}
	previewPath := filepath.Join(t.TempDir(), "title-pin.png")
	if path := os.Getenv("LOG_AGENT_PIN_PREVIEW"); path != "" {
		previewPath = path
	}
	preview, err := os.Create(previewPath)
	if err != nil {
		t.Fatal(err)
	}
	combined := image.NewRGBA(image.Rect(0, 0, before.Bounds().Dx()*3, before.Bounds().Dy()))
	for i, state := range []*image.RGBA{before, after, hover} {
		for y := 0; y < state.Bounds().Dy(); y++ {
			for x := 0; x < state.Bounds().Dx(); x++ {
				combined.SetRGBA(x+i*state.Bounds().Dx(), y, state.RGBAAt(x, y))
			}
		}
	}
	if err := png.Encode(preview, combined); err != nil {
		t.Fatal(err)
	}
	if err := preview.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("Native pin preview: %s", previewPath)
	procSendMessage.Call(p.button, 0x00F5, 0, 0)
	// Mouse press/release must toggle exactly once, and release outside cancels.
	procSendMessage.Call(p.button, 0x0201, 1, 10|(10<<16))
	procSendMessage.Call(p.button, 0x0202, 0, 10|(10<<16))
	if getStyle()&8 == 0 {
		t.Fatal("mouse click did not pin window")
	}
	procSendMessage.Call(p.button, 0x0201, 1, 10|(10<<16))
	procSendMessage.Call(p.button, 0x0202, 0, 0xffff|(0xffff<<16))
	if getStyle()&8 == 0 {
		t.Fatal("release outside unexpectedly toggled pin")
	}
	// External topmost changes are read back rather than cached by the button.
	pinSetWindowPos.Call(owner, ^uintptr(1), 0, 0, 0, 0, 0x13)
	procSendMessage.Call(p.button, 0x00F5, 0, 0)
	if getStyle()&8 == 0 {
		t.Fatal("external window state was ignored")
	}
	procShowWindow.Call(owner, 6) // SW_MINIMIZE
	if visible, _, _ := pinIsVisible.Call(p.button); visible != 0 {
		t.Fatal("pin remains visible after minimise")
	}
	procShowWindow.Call(owner, 4)
	procShowWindow.Call(owner, 0)
	if visible, _, _ := pinIsVisible.Call(p.button); visible != 0 {
		t.Fatal("pin remains visible after hide")
	}
	button := p.button
	p.close()
	p.close()
	if _, exists := captionPins.Load(owner); exists {
		t.Fatal("owner subclass state was not released")
	}
	if valid, _, _ := user32.NewProc("IsWindow").Call(button); valid != 0 {
		t.Fatal("pin window was not destroyed")
	}
}

// Paint into a native memory DC, so checking the button never captures user data.
func captureCaptionPin(t *testing.T, p *captionPin) *image.RGBA {
	t.Helper()
	var rect pinRect
	pinGetClientRect.Call(p.button, uintptr(unsafe.Pointer(&rect)))
	dc, _, _ := user32.NewProc("GetDC").Call(p.button)
	defer user32.NewProc("ReleaseDC").Call(p.button, dc)
	memory, _, _ := pinGDI.NewProc("CreateCompatibleDC").Call(dc)
	defer pinGDI.NewProc("DeleteDC").Call(memory)
	bitmap, _, _ := pinGDI.NewProc("CreateCompatibleBitmap").Call(dc, uintptr(rect.Right), uintptr(rect.Bottom))
	if memory == 0 || bitmap == 0 {
		t.Fatal("cannot allocate pin paint buffer")
	}
	old, _, _ := pinSelectObject.Call(memory, bitmap)
	defer func() { pinSelectObject.Call(memory, old); pinDeleteObject.Call(bitmap) }()
	procSendMessage.Call(p.button, 0x0318, memory, 4)
	im := image.NewRGBA(image.Rect(0, 0, int(rect.Right), int(rect.Bottom)))
	getPixel := pinGDI.NewProc("GetPixel")
	for y := 0; y < im.Bounds().Dy(); y++ {
		for x := 0; x < im.Bounds().Dx(); x++ {
			pixel, _, _ := getPixel.Call(memory, uintptr(x), uintptr(y))
			im.SetRGBA(x, y, color.RGBA{R: uint8(pixel), G: uint8(pixel >> 8), B: uint8(pixel >> 16), A: 255})
		}
	}
	return im
}
