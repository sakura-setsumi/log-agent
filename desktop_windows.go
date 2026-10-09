//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const desktopShortcutSupported = true

// The script reads its inputs from the environment so non-ASCII names and paths
// never pass through PowerShell's command-line parsing. It prints "created"
// when it wrote the shortcut and "kept" when one already pointed here.
const desktopShortcutScript = `$ErrorActionPreference = 'Stop'
$desktop = [Environment]::GetFolderPath('Desktop')
if (-not $desktop) { throw 'no desktop folder' }
$path = Join-Path $desktop ($env:LOG_AGENT_SHORTCUT_NAME + '.lnk')
$shell = New-Object -ComObject WScript.Shell
$exists = Test-Path -LiteralPath $path
if ($exists -and $shell.CreateShortcut($path).TargetPath -ieq $env:LOG_AGENT_SHORTCUT_TARGET) { 'kept'; exit 0 }
if (-not $exists -and $env:LOG_AGENT_SHORTCUT_ONLY_UPDATE -eq '1') { 'kept'; exit 0 }
$link = $shell.CreateShortcut($path)
$link.TargetPath = $env:LOG_AGENT_SHORTCUT_TARGET
$link.Arguments = '--open'
$link.WorkingDirectory = Split-Path -Parent $env:LOG_AGENT_SHORTCUT_TARGET
$link.IconLocation = $env:LOG_AGENT_SHORTCUT_TARGET + ',0'
$link.Description = 'Log Agent'
$link.Save()
'created'`

// writeDesktopShortcut creates the shortcut, or with onlyUpdate repoints one
// that already exists without recreating one the user deleted.
func writeDesktopShortcut(executable string, onlyUpdate bool) (bool, error) {
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", desktopShortcutScript)
	onlyUpdateValue := "0"
	if onlyUpdate {
		onlyUpdateValue = "1"
	}
	command.Env = append(os.Environ(),
		"LOG_AGENT_SHORTCUT_NAME="+desktopShortcutName,
		"LOG_AGENT_SHORTCUT_TARGET="+filepath.Clean(executable),
		"LOG_AGENT_SHORTCUT_ONLY_UPDATE="+onlyUpdateValue,
	)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	output, err := command.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)) == "created", nil
}

func openBrowser(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
