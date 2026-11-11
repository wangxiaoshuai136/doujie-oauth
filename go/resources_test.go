package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestResourceRoutesRejectDynamicAuthPaths(t *testing.T) {
	id := strings.Repeat("c", 32)
	putFlow(id, &loginFlow{Expires: time.Now().Add(time.Minute), Token: "live"})
	defer takeFlow(id)

	for _, path := range []string{"/qr.png", "/manual.html"} {
		resp, err := handleManagementResource(pluginapi.ManagementRequest{Method: "GET", Path: resourcePrefix + path})
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 404 {
			t.Fatalf("dynamic auth route %s must not be exposed, got %d", path, resp.StatusCode)
		}
	}
}

func TestManagementResourcesAreStaticOnly(t *testing.T) {
	result, err := handleMethod("management.register", nil)
	if err != nil {
		t.Fatal(err)
	}
	registration, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("unexpected registration: %T", result)
	}
	resources, ok := registration["resources"].([]map[string]any)
	if !ok {
		t.Fatalf("unexpected resources: %T", registration["resources"])
	}
	for _, item := range resources {
		path, _ := item["path"].(string)
		if path == "/qr.png" || path == "/manual.html" {
			t.Fatalf("dynamic auth route registered as public resource: %s", path)
		}
	}
	routes, ok := registration["routes"].([]map[string]any)
	if !ok || len(routes) == 0 {
		t.Fatalf("missing authenticated management routes: %T", registration["routes"])
	}
	for _, item := range routes {
		path, _ := item["Path"].(string)
		if !strings.HasPrefix(path, "/v0/management/doujie-oauth/") {
			t.Fatalf("management route outside the authenticated prefix: %s", path)
		}
	}
}

func TestManagementServesLogoAndDescriptor(t *testing.T) {
	for _, path := range []string{resourcePrefix + "/logo.png", resourcePrefix + "/doubao.png", resourcePrefix + "/dola.png", resourcePrefix + "/account.html", descriptorResourcePath} {
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
