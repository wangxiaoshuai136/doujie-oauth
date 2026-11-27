//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDarwinBrowserCandidates(t *testing.T) {
	candidates := browserExecutableCandidates("/Users/test")
	seen := map[string]bool{}
	for _, candidate := range candidates {
		seen[candidate] = true
	}
	for _, want := range []string{
		filepath.Join("/Applications", "Google Chrome.app", "Contents", "MacOS", "Google Chrome"),
		filepath.Join("/Applications", "Microsoft Edge.app", "Contents", "MacOS", "Microsoft Edge"),
		filepath.Join("/Applications", "Brave Browser.app", "Contents", "MacOS", "Brave Browser"),
	} {
		if !seen[want] {
			t.Fatalf("missing browser candidate %q in %#v", want, candidates)
		}
	}
}

func TestDarwinBrowserOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chromium")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOUJIE_BROWSER_PATH", path)
	got, err := resolveBrowserExecutableNamed("/Users/test", "auto")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(path) {
		t.Fatalf("browser path = %q", got)
	}
}

func TestDarwinRejectsSafari(t *testing.T) {
	if _, err := resolveBrowserExecutableNamed("/Users/test", "safari"); err == nil {
		t.Fatal("Safari must not be accepted for CDP browser sign-in")
	}
}
