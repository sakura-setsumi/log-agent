package main

import "testing"

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
