package main

import (
	"bytes"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestManagementServesQRCode(t *testing.T) {
	id := strings.Repeat("c", 32)
	putFlow(id, &loginFlow{Expires: time.Now().Add(time.Minute), QRPNG: []byte("PNG")})
	resp, err := handleManagementResource(pluginapi.ManagementRequest{Method: "GET", Path: resourcePrefix + "/qr.png"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || string(resp.Body) != "PNG" || resp.Headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected QR response: %d %q %v", resp.StatusCode, resp.Body, resp.Headers)
	}
	takeFlow(id)
	resp, _ = handleManagementResource(pluginapi.ManagementRequest{Method: "GET", Path: resourcePrefix + "/qr.png"})
	if resp.StatusCode != 404 {
		t.Fatalf("expired QR should be gone, got %d", resp.StatusCode)
	}
}

func TestManagementServesLogoAndDescriptor(t *testing.T) {
	for _, path := range []string{resourcePrefix + "/logo.png", resourcePrefix + "/doubao.png", resourcePrefix + "/dola.png", descriptorResourcePath} {
		resp, err := handleManagementResource(pluginapi.ManagementRequest{Method: "GET", Path: path})
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 || len(resp.Body) == 0 {
			t.Fatalf("unexpected response for %s: %d", path, resp.StatusCode)
		}
	}
	doubao, _ := handleManagementResource(pluginapi.ManagementRequest{Method: "GET", Path: resourcePrefix + "/doubao.png"})
	dola, _ := handleManagementResource(pluginapi.ManagementRequest{Method: "GET", Path: resourcePrefix + "/dola.png"})
	if bytes.Equal(doubao.Body, dola.Body) {
		t.Fatal("domestic and international entry icons must be distinct")
	}
	resp, _ := handleManagementResource(pluginapi.ManagementRequest{Method: "POST", Path: resourcePrefix + "/logo.png"})
	if resp.StatusCode != 405 {
		t.Fatalf("expected method not allowed, got %d", resp.StatusCode)
	}
}

func TestManualImportPageAndCompletion(t *testing.T) {
	token := strings.Repeat("a", 32)
	flowID := strings.Repeat("b", 32)
	putFlow(flowID, &loginFlow{Expires: time.Now().Add(time.Minute), ImportToken: token})
	defer takeFlow(flowID)

	path := resourcePrefix + "/manual.html"
	resp, err := handleManagementResource(pluginapi.ManagementRequest{Method: "GET", Path: path, Query: url.Values{"token": {token}}})
	if err != nil || resp.StatusCode != 200 || !bytes.Contains(resp.Body, []byte("重新读取桌面端")) || !bytes.Contains(resp.Body, []byte("action=desktop")) {
		t.Fatalf("manual page: %d %v %q", resp.StatusCode, err, resp.Body)
	}
	if err = completeDesktopImportFromCookies(token, map[string]string{"sessionid": "abc", "ttwid": "def"}); err != nil {
		t.Fatal(err)
	}
	if flow := manualImportFlow(token); flow == nil || flow.Imported == nil || flow.Imported.Provider != credentialProvider {
		t.Fatalf("manual import did not complete: %+v", flow)
	}
}
