package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestDecodeQRCode(t *testing.T) {
	raw := base64.StdEncoding.EncodeToString([]byte("PNG-BYTES-1234"))
	got, err := decodeQRCode(raw)
	if err != nil || string(got) != "PNG-BYTES-1234" {
		t.Fatalf("decodeQRCode: %q %v", got, err)
	}
	if _, err := decodeQRCode("https://example.com/qr.png"); err == nil {
		t.Fatal("remote QR should require an HTTP fetch")
	}
}

func TestAbsoluteBaseUsesHostOriginForCallbackURL(t *testing.T) {
	got := absoluteBase("", "http://127.0.0.1:8317/v8/management/oauth/callback")
	if got != "http://127.0.0.1:8317" {
		t.Fatalf("callback URL origin = %q", got)
	}
	got = absoluteBase("https://cpa.example.com/doujie", "http://127.0.0.1:8317/v8/management/oauth/callback")
	if got != "https://cpa.example.com/doujie" {
		t.Fatalf("public base override = %q", got)
	}
}

func TestMergeCookies(t *testing.T) {
	jar := map[string]string{}
	headers := http.Header{}
	headers.Add("Set-Cookie", "sessionid=abc; Path=/; HttpOnly")
	headers.Add("Set-Cookie", "ttwid=def; Path=/")
	mergeCookies(jar, headers)
	if jar["sessionid"] != "abc" || jar["ttwid"] != "def" {
		t.Fatalf("unexpected jar: %#v", jar)
	}
}

func TestParseAuth(t *testing.T) {
	c := credential{
		Type:     credentialProvider,
		Email:    "豆包账号",
		Cookies:  map[string]string{"sessionid": "s", "ttwid": "t"},
		DeviceID: "7000000000000000",
		WebID:    "7600000000000000000",
		FP:       "verify_x",
	}
	raw, _ := json.Marshal(c)
	resp := parseAuth(pluginapi.AuthParseRequest{Provider: entryID, RawJSON: raw, FileName: "doubao.json"})
	if !resp.Handled || resp.Auth.Provider != credentialProvider {
		t.Fatalf("expected handled auth: %+v", resp)
	}
	if _, exists := resp.Auth.Metadata["cookies"]; exists {
		t.Fatal("credential fields must not leak into host metadata")
	}
}

func TestAuthFromCookies(t *testing.T) {
	auth, err := authFromCookies(map[string]string{"sessionid": "session", "ttwid": "ttwid"}, siteCN)
	if err != nil {
		t.Fatal(err)
	}
	if auth.Provider != credentialProvider || auth.ID == "" || len(auth.StorageJSON) == 0 {
		t.Fatalf("unexpected auth: %+v", auth)
	}
	if auth.Metadata["oauth_provider"] != entryID || auth.Metadata["site"] != siteCN {
		t.Fatalf("unexpected domestic metadata: %+v", auth.Metadata)
	}
}

func TestAuthFromCookiesGlobal(t *testing.T) {
	auth, err := authFromCookies(map[string]string{"sessionid": "global-session", "ttwid": "ttwid"}, siteGlobal)
	if err != nil {
		t.Fatal(err)
	}
	var c credential
	if json.Unmarshal(auth.StorageJSON, &c) != nil || c.site() != siteGlobal {
		t.Fatalf("site not persisted: %+v", c)
	}
	if !strings.HasPrefix(auth.ID, "dola-") {
		t.Fatalf("unexpected global auth id: %s", auth.ID)
	}
	if auth.Metadata["oauth_provider"] != internationalEntryID || auth.Metadata["site"] != siteGlobal {
		t.Fatalf("unexpected global metadata: %+v", auth.Metadata)
	}
}

func TestParseGlobalAuthKeepsExplicitEntry(t *testing.T) {
	c := credential{
		Type: credentialProvider, Site: siteGlobal, Email: "豆姐国际版账号",
		Cookies: map[string]string{"sessionid": "s"}, DeviceID: "7000000000000000",
		WebID: "7600000000000000000", FP: "verify_x",
	}
	raw, _ := json.Marshal(c)
	resp := parseAuth(pluginapi.AuthParseRequest{Provider: internationalEntryID, RawJSON: raw, FileName: "dola.json"})
	if !resp.Handled || resp.Auth.Metadata["oauth_provider"] != internationalEntryID {
		t.Fatalf("expected global auth: %+v", resp)
	}
}

func TestAuthFromCookiesPersistsEntryMetadata(t *testing.T) {
	auth, err := authFromCookies(map[string]string{"sessionid": "persist"}, siteCN)
	if err != nil {
		t.Fatal(err)
	}
	var c credential
	if json.Unmarshal(auth.StorageJSON, &c) != nil {
		t.Fatal("storage json is invalid")
	}
	if c.Email != auth.Label || c.OAuthProvider != entryID {
		t.Fatalf("credential should carry its account label and entry: %+v", c)
	}
}

func TestImportDesktopAuthUsesVisibleEntry(t *testing.T) {
	auth, err := importDesktopAuth(nil, map[string]string{"sessionid": "desktop"})
	if err != nil {
		t.Fatal(err)
	}
	if auth.Metadata["oauth_provider"] != entryID {
		t.Fatalf("desktop import must be grouped under the domestic entry: %+v", auth.Metadata)
	}
	if auth.Metadata["legacy_provider"] != desktopEntryID {
		t.Fatalf("legacy provider marker missing: %+v", auth.Metadata)
	}
	if auth.Metadata["email"] == "" {
		t.Fatal("desktop import must carry an account label")
	}
}

func TestParseLegacyDesktopAuthUsesDomesticEntry(t *testing.T) {
	c := credential{
		Type: credentialProvider, Site: siteCN, Email: "豆姐账号 旧桌面端",
		OAuthProvider: desktopEntryID,
		Cookies:       map[string]string{"sessionid": "legacy"}, DeviceID: "7000000000000000",
		WebID: "7600000000000000000", FP: "verify_x",
	}
	raw, _ := json.Marshal(c)
	resp := parseAuth(pluginapi.AuthParseRequest{Provider: desktopEntryID, RawJSON: raw, FileName: "doubao-legacy.json"})
	if !resp.Handled || resp.Auth.Metadata["oauth_provider"] != entryID {
		t.Fatalf("legacy desktop credential should normalize to doubao: %+v", resp)
	}
}

func TestStartLoginNeverUsesQR(t *testing.T) {
	dir, err := os.MkdirTemp("", "dola-auth-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	defer func() {
		flowMu.Lock()
		flowStore = map[string]*loginFlow{}
		flowMu.Unlock()
	}()
	cases := []struct {
		provider string
		site     string
	}{
		{entryID, siteCN},
		{domesticQREntryID, siteCN},
		{internationalEntryID, siteGlobal},
		{internationalQREntryID, siteGlobal},
	}
	for _, tc := range cases {
		resp, err := startLogin(pluginapi.AuthLoginStartRequest{
			Provider: tc.provider,
			BaseURL:  "http://127.0.0.1:8317/v8/management/oauth/callback",
			Host:     pluginapi.HostConfigSummary{AuthDir: dir},
			Metadata: map[string]any{"auto_open": "false"},
		}, nil)
		if err != nil {
			t.Fatalf("%s: %v", tc.provider, err)
		}
		if resp.Metadata["site"] != tc.site || resp.Provider != credentialProvider || resp.State == "" {
			t.Fatalf("%s: unexpected start response: %+v", tc.provider, resp)
		}
		if strings.Contains(resp.URL, "/qr.html") || strings.Contains(resp.URL, "base64") {
			t.Fatalf("%s: QR fallback is not allowed: %q", tc.provider, resp.URL)
		}
		if !strings.Contains(resp.URL, "/account.html?state=") {
			t.Fatalf("%s: expected account login URL, got %q", tc.provider, resp.URL)
		}
		if resp.Metadata["source"] != "browser-login" || resp.Metadata["auto_open"] != false {
			t.Fatalf("%s: unexpected metadata: %+v", tc.provider, resp.Metadata)
		}
		poll, err := pollLogin(pluginapi.AuthLoginPollRequest{
			State: resp.State,
			Host:  pluginapi.HostConfigSummary{AuthDir: dir},
		}, nil)
		if err != nil {
			t.Fatalf("%s poll: %v", tc.provider, err)
		}
		if !strings.Contains(poll.Message, "登录") {
			t.Fatalf("%s: pending login should ask for the browser window: %+v", tc.provider, poll)
		}
	}
}

type sequenceSender struct {
	responses []pluginapi.HTTPResponse
	requests  []pluginapi.HTTPRequest
}

func (s *sequenceSender) Do(req pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
	s.requests = append(s.requests, req)
	if len(s.responses) == 0 {
		return pluginapi.HTTPResponse{StatusCode: http.StatusInternalServerError}, nil
	}
	resp := s.responses[0]
	s.responses = s.responses[1:]
	return resp, nil
}

func TestAccountLaunchRejectsInvalidState(t *testing.T) {
	resp := handleAccountLaunch(pluginapi.ManagementRequest{Method: http.MethodGet})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing state must be rejected: %+v", resp)
	}
	if err := launchPendingBrowser("invalid-state"); err == nil {
		t.Fatal("invalid state must not launch a browser")
	}
}

func TestSafeLoginRedirect(t *testing.T) {
	allowed := []string{
		"https://www.doubao.com/passport/ok",
		"https://passport.doubao.com/ok",
		"https://www.dola.com/passport/ok",
	}
	for _, raw := range allowed {
		site := siteCN
		if strings.Contains(raw, "dola.com") {
			site = siteGlobal
		}
		if _, ok := safeLoginRedirect(raw, site); !ok {
			t.Fatalf("expected redirect to be allowed: %s", raw)
		}
	}
	rejected := []string{
		"http://www.doubao.com/passport/ok",
		"https://evil.example.com/passport/ok",
		"https://doubao.com.evil.example/passport/ok",
		"https://www.doubao.com:8443/passport/ok",
		"https://user:pass@www.doubao.com/passport/ok",
		"https://www.dola.com/passport/ok",
	}
	for _, raw := range rejected {
		if _, ok := safeLoginRedirect(raw, siteCN); ok {
			t.Fatalf("expected redirect to be rejected: %s", raw)
		}
	}
}
