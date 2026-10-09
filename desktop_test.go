package main

import (
	"encoding/base64"
	"testing"
)

func TestEncodePowerShellCommandIsUTF16LEBase64(t *testing.T) {
	// "a\n中" in UTF-16LE is 61 00 0a 00 2d 4e.
	if got, want := encodePowerShellCommand("a\n中"), "YQAKAC1O"; got != want {
		t.Fatalf("encodePowerShellCommand = %q, want %q", got, want)
	}
}

func TestWantsBrowser(t *testing.T) {
	if !wantsBrowser([]string{"--open"}) || wantsBrowser(nil) || wantsBrowser([]string{"open"}) {
		t.Fatal("wantsBrowser should match only the --open flag")
	}
}

func TestParseDesktopShortcutOutputSkipsNoise(t *testing.T) {
	want := `C:\Users\EDY\Desktop\日志中枢.lnk`
	line := "created|" + base64.StdEncoding.EncodeToString([]byte(want))
	output := "#< CLIXML\r\n\ufeff" + line + "\r\n<Objs Version=\"1.1.0.1\"></Objs>"
	status, path, ok := parseDesktopShortcutOutput(output)
	if !ok || status != "created" || path != want {
		t.Fatalf("got %q %q %v", status, path, ok)
	}
	if _, _, ok := parseDesktopShortcutOutput("#< CLIXML\r\n<Objs/>"); ok {
		t.Fatal("output without a status line should not parse")
	}
}
