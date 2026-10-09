package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

// openBrowserFlag is passed by the desktop shortcut: a double-click should land
// on the dashboard, not on a console window the user then has to read a URL from.
const openBrowserFlag = "--open"

const desktopShortcutName = "日志中枢"

// desktopShortcutMarker records that the shortcut was created once, so a user
// who deletes it does not get it back on every start.
const desktopShortcutMarker = ".desktop-shortcut-created"

func wantsBrowser(args []string) bool {
	for _, arg := range args {
		if arg == openBrowserFlag {
			return true
		}
	}
	return false
}

// ensureDesktopShortcut creates a desktop shortcut to this executable the first
// time it runs, and repoints an existing one if the executable has moved.
// LOG_AGENT_NO_SHORTCUT=1 turns it off. Failures are logged, never fatal.
func ensureDesktopShortcut() {
	if !desktopShortcutSupported || strings.TrimSpace(os.Getenv("LOG_AGENT_NO_SHORTCUT")) == "1" {
		return
	}
	executable, err := os.Executable()
	if err != nil {
		return
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	// Builds from `go run` and `go test` are throwaway; a shortcut to them would break.
	if isGoRunExecutable(executable) || strings.HasSuffix(strings.ToLower(filepath.Base(executable)), ".test.exe") {
		return
	}
	marker := filepath.Join(configDir(), desktopShortcutMarker)
	_, statErr := os.Stat(marker)
	created, err := writeDesktopShortcut(executable, statErr == nil)
	if err != nil {
		log.Printf("desktop shortcut not created: %v", err)
		return
	}
	if created {
		log.Printf("desktop shortcut %q now opens %s", desktopShortcutName, executable)
	}
	if statErr != nil {
		_ = os.MkdirAll(filepath.Dir(marker), 0o755)
		_ = os.WriteFile(marker, []byte(executable+"\n"), 0o644)
	}
}
