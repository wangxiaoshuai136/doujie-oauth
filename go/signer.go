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
	payload, err := json.Marshal(signRequest{Method: req.Method, URL: req.URL, Headers: req.Headers, Body: string(req.Body)})
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

func newSigner(cfg config) requestSigner {
	if cfg.Signer == "external" && cfg.SignerURL != "" {
		return externalSigner{url: cfg.SignerURL}
	}
	return noSigner{}
}
