//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCookieMapFromCDPFiltersAndPrefersDoubao(t *testing.T) {
	cookies := []cdpCookie{
		{Name: "sessionid", Value: "bytedance-session", Domain: ".bytedance.com", Path: "/"},
		{Name: "sessionid", Value: "doubao-session", Domain: ".doubao.com", Path: "/"},
		{Name: "ttwid", Value: "tt", Domain: "www.doubao.com", Path: "/"},
		{Name: "not_allowed", Value: "x", Domain: "doubao.com.evil.test", Path: "/"},
		{Name: "empty", Value: "", Domain: ".doubao.com", Path: "/"},
	}

	got := cookieMapFromCDP(cookies, siteCN)
	if got["sessionid"] != "doubao-session" {
		t.Fatalf("sessionid = %q", got["sessionid"])
	}
	if got["ttwid"] != "tt" {
		t.Fatalf("ttwid = %q", got["ttwid"])
	}
	if _, ok := got["not_allowed"]; ok {
		t.Fatal("cookie from an unrelated domain was included")
	}
	if _, ok := got["empty"]; ok {
		t.Fatal("empty cookie was included")
	}
	if len(got) != 2 {
		t.Fatalf("cookie count = %d", len(got))
	}
}

func TestCookieMapFromCDPRequiresSessionID(t *testing.T) {
	got := cookieMapFromCDP([]cdpCookie{
		{Name: "ttwid", Value: "tt", Domain: ".doubao.com", Path: "/"},
	}, siteCN)
	if got == nil {
		t.Fatal("expected an empty map, got nil")
	}
	if len(got) != 0 {
		t.Fatalf("cookie count = %d", len(got))
	}
}

func TestCookieMapFromCDPPrefersSpecificDomain(t *testing.T) {
	got := cookieMapFromCDP([]cdpCookie{
		{Name: "sessionid", Value: "broad", Domain: ".doubao.com", Path: "/"},
		{Name: "sessionid", Value: "specific", Domain: "www.doubao.com", Path: "/"},
	}, siteCN)
	if got["sessionid"] != "specific" {
		t.Fatalf("sessionid = %q", got["sessionid"])
	}
}

func TestCookieMapFromCDPIncludesDola(t *testing.T) {
	got := cookieMapFromCDP([]cdpCookie{
		{Name: "sessionid", Value: "doubao-session", Domain: ".doubao.com", Path: "/"},
		{Name: "ttwid", Value: "doubao-tt", Domain: "www.doubao.com", Path: "/"},
		{Name: "sessionid", Value: "dola-session", Domain: ".dola.com", Path: "/"},
		{Name: "ttwid", Value: "dola-tt", Domain: "www.dola.com", Path: "/"},
	}, siteGlobal)
	if got["sessionid"] != "dola-session" || got["ttwid"] != "dola-tt" {
		t.Fatalf("unexpected Dola cookies: %#v", got)
	}
}

func TestCookieMapFromCDPKeepsPortalsSeparate(t *testing.T) {
	got := cookieMapFromCDP([]cdpCookie{
		{Name: "sessionid", Value: "dola-session", Domain: ".dola.com", Path: "/"},
	}, siteCN)
	if len(got) != 0 {
		t.Fatalf("domestic login accepted global cookies: %#v", got)
	}
}

func TestDevToolsWSURLForPort(t *testing.T) {
	tests := []struct {
		raw  string
		port int
		want bool
	}{
		{"ws://127.0.0.1:9222/devtools/page/abc", 9222, true},
		{"ws://localhost:9222/devtools/page/abc", 9222, true},
		{"ws://[::1]:9222/devtools/page/abc", 9222, true},
		{"ws://127.0.0.1:9223/devtools/page/abc", 9222, false},
		{"ws://example.test:9222/devtools/page/abc", 9222, false},
		{"http://127.0.0.1:9222/devtools/page/abc", 9222, false},
	}
	for _, test := range tests {
		if got := devToolsWSURLForPort(test.raw, test.port); got != test.want {
			t.Fatalf("devToolsWSURLForPort(%q, %d) = %v", test.raw, test.port, got)
		}
	}
}

func TestPickPageTargetPrefersDoubaoChat(t *testing.T) {
	targets := []devToolsTarget{
		{Type: "page", URL: "about:blank", WebSocketDebuggerURL: "ws://127.0.0.1:9222/devtools/page/blank"},
		{Type: "page", URL: "https://www.doubao.com/chat/", WebSocketDebuggerURL: "ws://127.0.0.1:9222/devtools/page/doubao"},
		{Type: "service_worker", URL: "https://www.doubao.com/", WebSocketDebuggerURL: "ws://127.0.0.1:9222/devtools/page/sw"},
	}
	got, ok := pickPageTarget(targets, 9222, siteCN)
	if !ok {
		t.Fatal("expected a page target")
	}
	if got.URL != "https://www.doubao.com/chat/" {
		t.Fatalf("target URL = %q", got.URL)
	}

	if _, ok := pickPageTarget([]devToolsTarget{
		{Type: "page", URL: "about:blank", WebSocketDebuggerURL: "ws://127.0.0.1:9223/devtools/page/blank"},
	}, 9222, siteCN); ok {
		t.Fatal("accepted a target on the wrong port")
	}
}

func TestPickPageTargetPrefersDolaChat(t *testing.T) {
	targets := []devToolsTarget{
		{Type: "page", URL: "https://www.doubao.com/chat/", WebSocketDebuggerURL: "ws://127.0.0.1:9222/devtools/page/doubao"},
		{Type: "page", URL: "https://www.dola.com/chat/", WebSocketDebuggerURL: "ws://127.0.0.1:9222/devtools/page/dola"},
	}
	got, ok := pickPageTarget(targets, 9222, siteGlobal)
	if !ok {
		t.Fatal("expected a page target")
	}
	if got.URL != "https://www.dola.com/chat/" {
		t.Fatalf("target URL = %q", got.URL)
	}
}

func TestMergeEnvironmentReplacesCaseInsensitive(t *testing.T) {
	base := []string{
		`Path=C:\Windows`,
		`TEMP=old`,
		`temp=older`,
		`KEEP=1`,
	}
	got := mergeEnvironment(base, map[string]string{
		"TEMP":        "new",
		"USERPROFILE": `C:\Users\test`,
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
	if values["USERPROFILE"] != `C:\Users\test` {
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
			return `C:\Users\test\AppData\Local`
		default:
			return ""
		}
	}
	candidates := browserExecutableCandidates(`C:\Users\test`, getenv)
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
	got, err := resolveBrowserExecutableForUser(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(path) {
		t.Fatalf("browser path = %q", got)
	}
}
