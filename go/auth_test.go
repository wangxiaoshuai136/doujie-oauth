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

func TestStartLoginGlobalUsesDolaFlow(t *testing.T) {
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
	png := base64.StdEncoding.EncodeToString([]byte("PNG-BYTES-1234"))
	sender := &sequenceSender{responses: []pluginapi.HTTPResponse{
		{StatusCode: http.StatusOK, Headers: http.Header{"Set-Cookie": {"ttwid=tt; Path=/"}}, Body: []byte("<html>ok</html>")},
		{StatusCode: http.StatusOK, Headers: http.Header{"Set-Cookie": {"passport_csrf_token=csrf; Path=/"}}, Body: []byte(`{"data":{"passport_csrf_token":"csrf"}}`)},
		{StatusCode: http.StatusOK, Body: []byte(`{"data":{"token":"token","qrcode":"` + png + `"}}`)},
	}}
	resp, err := startLogin(pluginapi.AuthLoginStartRequest{
		Provider: internationalEntryID,
		BaseURL:  "http://127.0.0.1:8317/v8/management/oauth/callback",
		Host:     pluginapi.HostConfigSummary{AuthDir: dir},
	}, sender)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Metadata["site"] != siteGlobal || resp.Provider != credentialProvider || resp.State == "" {
		t.Fatalf("unexpected start response: %+v", resp)
	}
	if len(sender.requests) != 3 {
		t.Fatalf("expected three upstream calls, got %d", len(sender.requests))
	}
	for _, req := range sender.requests {
		if !strings.HasPrefix(req.URL, "https://www.dola.com/") {
			t.Fatalf("unexpected global URL: %s", req.URL)
		}
	}
	if !strings.Contains(sender.requests[1].URL, "aid=495671") || !strings.Contains(sender.requests[2].URL, "aid=495671") {
		t.Fatalf("global AID missing: %v", sender.requests)
	}
	if !strings.Contains(sender.requests[2].Headers.Get("Cookie"), "passport_csrf_token=csrf") {
		t.Fatalf("CSRF cookie not forwarded: %q", sender.requests[2].Headers.Get("Cookie"))
	}
	if resp.URL != "http://127.0.0.1:8317"+resourcePrefix+"/qr.png" {
		t.Fatalf("QR URL = %q", resp.URL)
	}
}
