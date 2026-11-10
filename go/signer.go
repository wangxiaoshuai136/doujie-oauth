package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type requestSigner interface {
	Sign(req pluginapi.HTTPRequest, sender httpSender) (pluginapi.HTTPRequest, error)
}

type noSigner struct{}

func (noSigner) Sign(req pluginapi.HTTPRequest, _ httpSender) (pluginapi.HTTPRequest, error) {
	return req, nil
}

type externalSigner struct{ url string }

type signRequest struct {
	Method  string              `json:"method"`
	URL     string              `json:"url"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    string              `json:"body,omitempty"`
}

type signResponse struct {
	Headers map[string]string `json:"headers"`
}

func (s externalSigner) Sign(req pluginapi.HTTPRequest, sender httpSender) (pluginapi.HTTPRequest, error) {
	if sender == nil {
		return req, errors.New("signer service unavailable")
	}
	if !trustedSignerURL(s.url) {
		return req, errors.New("signer URL is not trusted")
	}
	payload, err := json.Marshal(signRequest{Method: req.Method, URL: req.URL, Headers: safeSignerHeaders(req.Headers), Body: string(req.Body)})
	if err != nil {
		return req, errors.New("cannot encode signer request")
	}
	resp, err := sender.Do(pluginapi.HTTPRequest{
		Method:  http.MethodPost,
		URL:     s.url,
		Headers: http.Header{"Content-Type": {"application/json"}, "Accept": {"application/json"}},
		Body:    payload,
	})
	if err != nil {
		return req, errors.New("signer service unavailable")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return req, errors.New("signer service rejected the request")
	}
	if len(resp.Body) > 1<<20 {
		return req, errors.New("signer response too large")
	}
	var out signResponse
	if json.Unmarshal(resp.Body, &out) != nil {
		return req, errors.New("signer returned an invalid response")
	}
	headers := req.Headers.Clone()
	if headers == nil {
		headers = http.Header{}
	}
	for name, value := range out.Headers {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "", "host", "content-length", "connection", "cookie":
			continue
		}
		headers.Set(name, value)
	}
	req.Headers = headers
	return req, nil
}

// safeSignerHeaders forwards only the target request metadata a signer needs.
// Session cookies, authorization headers and CSRF tokens stay inside the plugin.
func safeSignerHeaders(in http.Header) map[string][]string {
	out := map[string][]string{}
	for _, name := range []string{"Content-Type", "Accept", "User-Agent", "Agw-Js-Conv"} {
		if values := in.Values(name); len(values) > 0 {
			out[name] = append([]string(nil), values...)
		}
	}
	return out
}

// trustedSignerURL requires HTTPS for remote signers and allows plain HTTP only
// on the loopback interface, where the operator controls the trust boundary.
func trustedSignerURL(raw string) bool {
	u, ok := httpBaseURL(raw)
	if !ok || u.User != nil {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	switch strings.ToLower(u.Hostname()) {
	case "127.0.0.1", "::1", "localhost":
		return true
	default:
		return false
	}
}

func newSigner(cfg config) requestSigner {
	if cfg.Signer == "external" && cfg.SignerURL != "" {
		return externalSigner{url: cfg.SignerURL}
	}
	return noSigner{}
}
