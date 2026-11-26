package main

import (
	_ "embed"
	"encoding/json"
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
	if req.Path == resourcePrefix+"/account-launch" {
		return handleAccountLaunch(req), nil
	}
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

func handleAccountLaunch(req pluginapi.ManagementRequest) pluginapi.ManagementResponse {
	headers := http.Header{
		"Content-Type":           {"application/json; charset=utf-8"},
		"Cache-Control":          {"no-store"},
		"X-Content-Type-Options": {"nosniff"},
		"Referrer-Policy":        {"no-referrer"},
	}
	state := req.Query.Get("state")
	if state == "" {
		body, _ := json.Marshal(map[string]any{"status": "error", "error": "缺少登录状态，请回到面板重新开始"})
		return pluginapi.ManagementResponse{StatusCode: http.StatusBadRequest, Headers: headers, Body: body}
	}
	if err := launchPendingBrowser(state); err != nil {
		body, _ := json.Marshal(map[string]any{"status": "error", "error": err.Error()})
		return pluginapi.ManagementResponse{StatusCode: http.StatusBadRequest, Headers: headers, Body: body}
	}
	body, _ := json.Marshal(map[string]any{"status": "ok"})
	return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: headers, Body: body}
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
<style>:root{color-scheme:dark}body{font-family:system-ui,-apple-system,"Segoe UI",sans-serif;background:#0f172a;color:#e2e8f0;margin:0;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px}main{width:min(560px,100%);background:#1e293b;border-radius:18px;padding:34px;box-shadow:0 24px 70px rgba(0,0,0,.45)}h1{font-size:24px;margin:0 0 12px}p{line-height:1.7;color:#94a3b8}.note{border-left:3px solid #38bdf8;background:#13233a;padding:12px 14px;border-radius:8px;color:#bae6fd}.small{font-size:13px;color:#64748b}button{margin-top:18px;border:0;border-radius:10px;background:#2563eb;color:#fff;font-size:15px;padding:11px 18px;cursor:pointer}button:disabled{background:#334155;color:#94a3b8;cursor:default}</style></head>
<body><main><h1>请在账号登录窗口完成登录</h1>
<p id="status">正在启动登录窗口…</p>
<div class="note">只支持手机号、密码或短信验证码登录，不会显示二维码。登录成功后 CPA 会自动读取该窗口的会话。</div>
<button id="retry" type="button" hidden>重新打开登录窗口</button>
<p class="small">此链接会在 5 分钟后失效；关闭本页不会中断已经打开的登录窗口。</p>
</main>
<script>
(function () {
  var params = new URLSearchParams(location.search);
  var state = params.get('state') || '';
  var status = document.getElementById('status');
  var retry = document.getElementById('retry');
  function launch() {
    if (!state) {
      status.textContent = '缺少登录状态，请回到面板重新开始。';
      return;
    }
    retry.hidden = true;
    status.textContent = '正在启动登录窗口…';
    fetch('account-launch?state=' + encodeURIComponent(state), { cache: 'no-store' })
      .then(function (resp) { return resp.json().then(function (data) { return { ok: resp.ok, data: data }; }); })
      .then(function (result) {
        if (result.ok && result.data && result.data.status === 'ok') {
          status.textContent = '登录窗口已打开，请在其中使用手机号、密码或短信验证码完成登录。';
          return;
        }
        status.textContent = '启动失败：' + ((result.data && result.data.error) || '未知错误');
        retry.hidden = false;
      })
      .catch(function (err) {
        status.textContent = '启动失败：' + err;
        retry.hidden = false;
      });
  }
  retry.addEventListener('click', launch);
  launch();
})();
</script>
</body></html>`
