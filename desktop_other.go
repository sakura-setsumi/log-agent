//go:build !windows

package main

import (
	"os/exec"
	"runtime"
)

const desktopShortcutSupported = false

func writeDesktopShortcut(string, bool) (bool, error) { return false, nil }

func openBrowser(url string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", url).Start()
	}
	return exec.Command("xdg-open", url).Start()
}
