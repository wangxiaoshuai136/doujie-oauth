//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeEnvironmentReplacesCaseInsensitive(t *testing.T) {
	base := []string{
		`Path=C:\Windows`,
		`TEMP=old`,
		`temp=older`,
		`KEEP=1`,
	}
	got := mergeEnvironment(base, map[string]string{
		"TEMP":        "new",
		"USERPROFILE": `C:\Users	est`,
	})

	values := map[string]string{}
	tempCount := 0
	for _, entry := range got {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		key := strings.ToUpper(name)
		values[key] = value
		if key == "TEMP" {
			tempCount++
		}
	}
	if values["TEMP"] != "new" {
		t.Fatalf("TEMP = %q", values["TEMP"])
	}
	if tempCount != 1 {
		t.Fatalf("TEMP entries = %d", tempCount)
	}
	if values["PATH"] != `C:\Windows` {
		t.Fatalf("PATH = %q", values["PATH"])
	}
	if values["USERPROFILE"] != `C:\Users	est` {
		t.Fatalf("USERPROFILE = %q", values["USERPROFILE"])
	}
	if values["KEEP"] != "1" {
		t.Fatalf("KEEP = %q", values["KEEP"])
	}
}

func TestBrowserExecutableCandidatesUseChromeOnly(t *testing.T) {
	getenv := func(name string) string {
		switch name {
		case "ProgramFiles":
			return `C:\Program Files`
		case "ProgramFiles(x86)":
			return `C:\Program Files (x86)`
		case "LOCALAPPDATA":
			return `C:\Users	est\AppData\Local`
		default:
			return ""
		}
	}
	candidates := browserExecutableCandidates(`C:\Users	est`, getenv)
	for _, candidate := range candidates {
		lower := strings.ToLower(candidate)
		if !strings.Contains(lower, `google\chrome`) {
			t.Fatalf("non-Chrome fallback candidate: %s", candidate)
		}
	}
	if len(candidates) == 0 {
		t.Fatal("missing Chrome candidates")
	}
}

func TestParseBrowserCommand(t *testing.T) {
	for command, want := range map[string]string{
		`"C:\Program Files\Google\Chrome\Application\chrome.exe" --single-argument %1`: `C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Chrome\chrome.exe --single-argument %1`:                                    `C:\Chrome\chrome.exe`,
	} {
		got, err := parseBrowserCommand(command)
		if err != nil || got != want {
			t.Fatalf("parseBrowserCommand(%q) = %q, %v", command, got, err)
		}
	}
}

func TestIsChromiumBrowser(t *testing.T) {
	for _, path := range []string{`C:\Chrome\chrome.exe`, `C:\Edge\msedge.exe`, `C:\Brave\brave.exe`} {
		if !isChromiumBrowser(path) {
			t.Fatalf("expected Chromium browser: %s", path)
		}
	}
	if isChromiumBrowser(`C:\Firefox\firefox.exe`) {
		t.Fatal("Firefox must not be treated as Chromium")
	}
}

func TestResolveBrowserExecutableOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom-browser.exe")
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOUJIE_BROWSER_PATH", path)
	got, err := resolveBrowserExecutableForUser(t.TempDir(), 0, "auto")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(path) {
		t.Fatalf("browser path = %q", got)
	}
}
