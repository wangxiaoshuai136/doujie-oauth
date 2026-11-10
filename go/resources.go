package main

import (
	_ "embed"
	"net/http"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const pluginStoreID = "doujie-oauth"
const resourcePrefix = "/v0/resource/plugins/" + pluginStoreID
const publicLogoBaseURL = "https://raw.githubusercontent.com/wangxiaoshuai136/doujie-oauth/refs/heads/main/assets/"

func resourceAssetURL(name string) string {
	switch name {
	case "logo.png", "doubao.png", "dola.png":
		return publicLogoBaseURL + name
	default:
		return ""
	}
}

//go:embed assets/logo.png
var logoPNG []byte

//go:embed assets/doubao.png
var doubaoPNG []byte

//go:embed assets/dola.png
var dolaPNG []byte

func handleManagementResource(req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	if req.Method != http.MethodGet {
		return pluginapi.ManagementResponse{
			StatusCode: http.StatusMethodNotAllowed,
			Headers:    http.Header{"Allow": {http.MethodGet}},
			Body:       []byte("method not allowed"),
		}, nil
	}
	return handleManagementGet(req)
}

func handleManagementGet(req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	contentType := "image/png"
	cacheControl := "public, max-age=86400"
	var body []byte
	switch {
	case req.Path == resourcePrefix+"/logo.png":
		body = logoPNG
	case req.Path == resourcePrefix+"/doubao.png":
		body = doubaoPNG
	case req.Path == resourcePrefix+"/dola.png":
		body = dolaPNG
	case req.Path == descriptorResourcePath:
		body = []byte(descriptorJSON)
		contentType = "application/json; charset=utf-8"
		cacheControl = "no-cache"
	case req.Path == resourcePrefix+"/imported.html":
		body = []byte(desktopImportedPage)
		contentType = "text/html; charset=utf-8"
		cacheControl = "no-store"
	case req.Path == resourcePrefix+"/account.html":
		body = []byte(browserAccountPage)
		contentType = "text/html; charset=utf-8"
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

const browserAccountPage = `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>豆姐账号登录</title>
<style>:root{color-scheme:dark}body{font-family:system-ui,-apple-system,"Segoe UI",sans-serif;background:#0f172a;color:#e2e8f0;margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px}main{width:min(560px,100%);background:#1e293b;border-radius:18px;padding:34px;box-shadow:0 24px 70px rgba(0,0,0,.45)}h1{font-size:24px;margin:0 0 12px}p{line-height:1.7;color:#94a3b8}a{color:#38bdf8}.note{border-left:3px solid #38bdf8;background:#13233a;padding:12px 14px;border-radius:8px;color:#bae6fd}.small{font-size:13px;color:#64748b}</style></head>
<body><main><h1>请在浏览器中完成登录</h1>
<p>插件已使用系统默认浏览器打开一个独立窗口。你可以使用手机号验证码、账号密码或飞书账号登录，不需要扫码。</p>
<div class="note">登录成功后，CPA 会自动读取该浏览器会话并完成授权。不要在这个窗口里打开其他账号的私人页面。</div>
<p><a href="https://www.doubao.com/chat/" target="_blank" rel="noreferrer">如果浏览器没有自动弹出，点这里打开豆包</a></p>
<p class="small">此窗口会在 5 分钟后失效；关闭本页不会中断已打开浏览器的登录。</p>
</main></body></html>`
