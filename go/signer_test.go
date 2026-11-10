package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type fakeSender struct {
	response pluginapi.HTTPResponse
	calls    int
	lastReq  pluginapi.HTTPRequest
}

func (f *fakeSender) Do(req pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
	f.calls++
	f.lastReq = req
	return f.response, nil
}

func TestExternalSignerMergesHeaders(t *testing.T) {
	sender := &fakeSender{response: pluginapi.HTTPResponse{StatusCode: 200, Body: []byte(`{"headers":{"X-Bogus":"sig","Host":"evil","Cookie":"other"}}`)}}
	req, err := externalSigner{url: "http://127.0.0.1:8787/sign"}.Sign(pluginapi.HTTPRequest{
		Method:  http.MethodPost,
		URL:     "https://www.doubao.com/x",
		Headers: http.Header{"Cookie": {"sessionid=s"}, "Authorization": {"Bearer secret"}, "Content-Type": {"application/json"}},
		Body:    []byte(`{"prompt":"private"}`),
	}, sender)
	if err != nil {
		t.Fatal(err)
	}
	if sender.calls != 1 {
		t.Fatalf("expected one signer call, got %d", sender.calls)
	}
	if req.Headers.Get("X-Bogus") != "sig" {
		t.Fatalf("missing signer header: %v", req.Headers)
	}
	if req.Headers.Get("Host") == "evil" || req.Headers.Get("Cookie") != "sessionid=s" {
		t.Fatalf("restricted headers must be preserved: %v", req.Headers)
	}
	if strings.Contains(string(sender.lastReq.Body), "sessionid") || strings.Contains(string(sender.lastReq.Body), "Bearer secret") {
		t.Fatalf("signer payload leaked credentials: %s", sender.lastReq.Body)
	}
	var forwarded signRequest
	if err = json.Unmarshal(sender.lastReq.Body, &forwarded); err != nil {
		t.Fatal(err)
	}
	if forwarded.Headers["Cookie"] != nil || forwarded.Headers["Authorization"] != nil {
		t.Fatalf("signer received restricted headers: %v", forwarded.Headers)
	}
	if forwarded.Headers["Content-Type"] == nil {
		t.Fatalf("signer should receive non-sensitive target metadata: %v", forwarded.Headers)
	}
}

func TestSignerRejectsUntrustedURL(t *testing.T) {
	for _, raw := range []string{"http://signer.example.com/sign", "http://10.0.0.5/sign", "ftp://127.0.0.1/sign"} {
		if _, err := (externalSigner{url: raw}).Sign(pluginapi.HTTPRequest{}, &fakeSender{}); err == nil {
			t.Fatalf("untrusted signer URL accepted: %s", raw)
		}
	}
	for _, raw := range []string{"https://signer.example.com/sign", "http://127.0.0.1:8787/sign", "http://localhost/sign", "http://[::1]:8787/sign"} {
		if !trustedSignerURL(raw) {
			t.Fatalf("trusted signer URL rejected: %s", raw)
		}
	}
}

func TestSignerRejectsInvalidResponse(t *testing.T) {
	sender := &fakeSender{response: pluginapi.HTTPResponse{StatusCode: 200, Body: []byte(`{"headers":[]}`)}}
	if _, err := (externalSigner{url: "http://127.0.0.1/sign"}).Sign(pluginapi.HTTPRequest{}, sender); err == nil {
		t.Fatal("expected invalid signer response to fail")
	}
}

func TestNoSignerPassthrough(t *testing.T) {
	original := pluginapi.HTTPRequest{Headers: http.Header{"A": {"b"}}}
	got, err := noSigner{}.Sign(original, nil)
	if err != nil || got.Headers.Get("A") != "b" {
		t.Fatalf("unexpected passthrough: %+v %v", got, err)
	}
}

func TestBuildRequest(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	c := credential{Type: credentialProvider, Cookies: map[string]string{"sessionid": "s", "msToken": "m"}, DeviceID: "7000000000000000", WebID: "7600000000000000000", FP: "verify_x"}
	storage, _ := json.Marshal(c)
	payload := []byte(`{"model":"doubao-seed-1.6-thinking","messages":[{"role":"user","content":"你好"}]}`)
	plan, err := buildRequest(pluginapi.ExecutorRequest{Payload: payload, Model: "doubao-seed-1.6-thinking", StorageJSON: storage}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.HTTP.Method != http.MethodPost || !strings.Contains(plan.HTTP.URL, "/samantha/chat/completion?") || !strings.Contains(plan.HTTP.URL, "aid=582478") || !strings.Contains(plan.HTTP.URL, "msToken=m") {
		t.Fatalf("unexpected URL: %s", plan.HTTP.URL)
	}
	if plan.HTTP.Headers.Get("Cookie") != "msToken=m; sessionid=s" {
		t.Fatalf("unexpected cookie header: %q", plan.HTTP.Headers.Get("Cookie"))
	}
	if !plan.DeepThink {
		t.Fatal("thinking model should enable deep think")
	}
	if !strings.Contains(string(plan.HTTP.Body), "你好") || !strings.Contains(string(plan.HTTP.Body), "bot_id") {
		t.Fatalf("unexpected body: %s", plan.HTTP.Body)
	}
}

func TestBuildRequestGlobalUsesDolaRouting(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	c := credential{Type: credentialProvider, Site: siteGlobal, Cookies: map[string]string{"sessionid": "s"}, DeviceID: "7000000000000000", WebID: "7600000000000000000", FP: "verify_x"}
	storage, _ := json.Marshal(c)
	payload := []byte(`{"model":"doubao","messages":[{"role":"user","content":"hello"}]}`)
	plan, err := buildRequest(pluginapi.ExecutorRequest{Payload: payload, Model: "doubao", StorageJSON: storage}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plan.HTTP.URL, "https://www.dola.com/samantha/chat/completion?") || !strings.Contains(plan.HTTP.URL, "aid=495671") {
		t.Fatalf("unexpected global URL: %s", plan.HTTP.URL)
	}
	if strings.Contains(plan.HTTP.URL, "region=CN") {
		t.Fatalf("global request must not force CN region: %s", plan.HTTP.URL)
	}
	if !strings.Contains(string(plan.HTTP.Body), "7339470689562525703") {
		t.Fatalf("global bot id missing: %s", plan.HTTP.Body)
	}
}
