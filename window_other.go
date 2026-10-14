//go:build !windows

package main

import (
	"errors"
	"net/http"
)

// The desktop window is only implemented on Windows (WebView2).
const desktopShortcutSupported = false

func writeDesktopShortcut(string, bool) (string, string, error) { return "", "", nil }

func runDesktop(http.Handler) error {
	return errors.New("the desktop window is only available on Windows")
}
