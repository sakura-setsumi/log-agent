package main

import (
	"encoding/base64"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

// openBrowserFlag is passed by the desktop shortcut: a double-click should land
// on the dashboard, not on a console window the user then has to read a URL from.
const openBrowserFlag = "--open"

const desktopShortcutName = "日志中枢"

// desktopShortcutMarker records that the shortcut was created once, so a user
// who deletes it does not get it back on every start. It is written only after
// the shortcut is confirmed on the desktop. (An earlier build wrote
// ".desktop-shortcut-created" even when nothing was created, so that name is
// deliberately not reused.)
const desktopShortcutMarker = ".desktop-shortcut"

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
	status, path, err := writeDesktopShortcut(executable, statErr == nil)
	if err != nil {
		log.Printf("desktop shortcut not created: %v", err)
		return
	}
	switch status {
	case "created":
		log.Printf("desktop shortcut created: %s", path)
	case "updated":
		log.Printf("desktop shortcut now opens %s: %s", executable, path)
	case "kept":
		log.Printf("desktop shortcut: %s", path)
	case "removed":
		log.Printf("desktop shortcut was deleted, not recreating it; remove %s to get it back", marker)
	}
	if statErr != nil && status != "removed" {
		_ = os.MkdirAll(filepath.Dir(marker), 0o755)
		_ = os.WriteFile(marker, []byte(executable+"\n"), 0o644)
	}
}

// encodePowerShellCommand encodes a script for powershell -EncodedCommand,
// which takes base64 of the UTF-16LE text.
func encodePowerShellCommand(script string) string {
	units := utf16.Encode([]rune(script))
	raw := make([]byte, len(units)*2)
	for i, unit := range units {
		raw[i*2] = byte(unit)
		raw[i*2+1] = byte(unit >> 8)
	}
	return base64.StdEncoding.EncodeToString(raw)
}
