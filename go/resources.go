package main

import (
	_ "embed"
	"net/http"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const pluginStoreID = "omc-doubao-oauth"
const resourcePrefix = "/v0/resource/plugins/" + pluginStoreID
const publicLogoBaseURL = "https://raw.githubusercontent.com/paul19910215-sketch/omc-doubao-oauth/refs/heads/main/assets/"

func resourceAssetURL(name string) string {
	switch name {
	case "logo.png", "doubao.png":
		return publicLogoBaseURL + name
	default:
		return ""
	}
}

//go:embed assets/logo.png
var logoPNG []byte

func handleManagementResource(req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	if req.Method != http.MethodGet {
		return pluginapi.ManagementResponse{
			StatusCode: http.StatusMethodNotAllowed,
			Headers:    http.Header{"Allow": {http.MethodGet}},
			Body:       []byte("method not allowed"),
		}, nil
	}
	contentType := "image/png"
	cacheControl := "public, max-age=86400"
	var body []byte
	switch {
	case req.Path == resourcePrefix+"/logo.png" || req.Path == resourcePrefix+"/doubao.png":
		body = logoPNG
	case req.Path == omcResourcePath:
		body = []byte(omcDescriptorJSON)
		contentType = "application/json; charset=utf-8"
		cacheControl = "no-cache"
	case req.Path == resourcePrefix+"/qr.png":
		flow := newestFlow()
		if flow == nil || len(flow.QRPNG) == 0 {
			return pluginapi.ManagementResponse{StatusCode: http.StatusNotFound, Body: []byte("not found")}, nil
		}
		body = flow.QRPNG
		cacheControl = "no-store"
	default:
		return pluginapi.ManagementResponse{StatusCode: http.StatusNotFound, Body: []byte("not found")}, nil
	}
	return pluginapi.ManagementResponse{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"Content-Type":           {contentType},
			"Cache-Control":          {cacheControl},
			"X-Content-Type-Options": {"nosniff"},
		},
		Body: append([]byte(nil), body...),
	}, nil
}

