package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
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
	auth, err := authFromCookies(map[string]string{"sessionid": "session", "ttwid": "ttwid"})
	if err != nil {
		t.Fatal(err)
	}
	if auth.Provider != credentialProvider || auth.ID == "" || len(auth.StorageJSON) == 0 {
		t.Fatalf("unexpected auth: %+v", auth)
	}
}
