package main

import (
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
	for _, path := range []string{resourcePrefix + "/logo.png", omcResourcePath} {
		resp, err := handleManagementResource(pluginapi.ManagementRequest{Method: "GET", Path: path})
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 || len(resp.Body) == 0 {
			t.Fatalf("unexpected response for %s: %d", path, resp.StatusCode)
		}
	}
	resp, _ := handleManagementResource(pluginapi.ManagementRequest{Method: "POST", Path: resourcePrefix + "/logo.png"})
	if resp.StatusCode != 405 {
		t.Fatalf("expected method not allowed, got %d", resp.StatusCode)
	}
}
