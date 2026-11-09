package main

import (
	_ "embed"
	"net/http"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const pluginStoreID = "omc-doubao-oauth"
const resourcePrefix = "/v0/resource/plugins/" + pluginStoreID
const publicLogoBaseURL = "https://raw.githubusercontent.com/wangxiaoshuai136/omc-doubao-oauth/refs/heads/main/assets/"

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
	case req.Path == resourcePrefix+"/imported.html":
		body = []byte(desktopImportedPage)
		contentType = "text/html; charset=utf-8"
		cacheControl = "no-store"
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

const desktopImportedPage = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8">
<title>豆姐桌面端账号已导入</title>
<style>body{font-family:system-ui,-apple-system,"Segoe UI",sans-serif;background:#0f172a;color:#e2e8f0;display:flex;align-items:center;justify-content:center;height:100vh;margin:0}main{max-width:520px;padding:40px;border-radius:16px;background:#1e293b;box-shadow:0 20px 60px rgba(0,0,0,.4)}h1{font-size:22px;margin:0 0 12px}p{line-height:1.7;color:#94a3b8}code{color:#38bdf8}</style></head>
<body><main><h1>已从本地桌面端导入账号</h1>
<p>插件已读取本机桌面客户端的登录态并写入 CPA。可以关闭此页，回到面板查看额度卡。</p>
<p>如果没有看到账号，请确认桌面客户端已登录后重新点击该入口。</p></main></body></html>`
