//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const desktopShortcutSupported = true

// The script reads its inputs from the environment so non-ASCII names and paths
// never pass through PowerShell's command-line parsing, and it is handed over
// with -EncodedCommand so its own quotes and newlines cannot be mangled either.
// Its last output line is "<status>|<shortcut path>".
const desktopShortcutScript = `$ErrorActionPreference = 'Stop'
$desktop = [Environment]::GetFolderPath('Desktop')
if (-not $desktop) { throw 'no desktop folder' }
$path = Join-Path $desktop ($env:LOG_AGENT_SHORTCUT_NAME + '.lnk')
$shell = New-Object -ComObject WScript.Shell
$exists = Test-Path -LiteralPath $path
if ($exists -and $shell.CreateShortcut($path).TargetPath -ieq $env:LOG_AGENT_SHORTCUT_TARGET) { 'kept|' + $path; exit 0 }
if (-not $exists -and $env:LOG_AGENT_SHORTCUT_ONLY_UPDATE -eq '1') { 'removed|' + $path; exit 0 }
$link = $shell.CreateShortcut($path)
$link.TargetPath = $env:LOG_AGENT_SHORTCUT_TARGET
$link.Arguments = '--open'
$link.WorkingDirectory = Split-Path -Parent $env:LOG_AGENT_SHORTCUT_TARGET
$link.IconLocation = $env:LOG_AGENT_SHORTCUT_TARGET + ',0'
$link.Description = 'Log Agent'
$link.Save()
if (-not (Test-Path -LiteralPath $path)) { throw ('shortcut was not written: ' + $path) }
if ($exists) { 'updated|' + $path } else { 'created|' + $path }`

// writeDesktopShortcut creates the shortcut, or with onlyUpdate repoints one
// that already exists without recreating one the user deleted.
func writeDesktopShortcut(executable string, onlyUpdate bool) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePowerShellCommand(desktopShortcutScript))
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
	text := strings.TrimSpace(string(output))
	if ctx.Err() != nil {
		return "", "", fmt.Errorf("PowerShell did not finish within a minute")
	}
	if err != nil {
		return "", "", fmt.Errorf("%v: %s", err, text)
	}
	lines := strings.Split(text, "\n")
	status, path, ok := strings.Cut(strings.TrimSpace(lines[len(lines)-1]), "|")
	if !ok {
		return "", "", fmt.Errorf("unexpected PowerShell output: %q", text)
	}
	return status, path, nil
}

func openBrowser(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
