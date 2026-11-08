<p align="center">
  <img src="assets/logo.png" width="96" height="96" alt="OMC Doubao OAuth logo">
</p>

<h1 align="center">OMC Doubao OAuth</h1>

<p align="center">
  非官方豆包（Doubao）提供商插件 · 为
  <a href="https://github.com/router-for-me/CLIProxyAPI">CLIProxyAPI</a>
  提供扫码登录与豆包对话模型
</p>

<p align="center">
  <a href="https://github.com/paul19910215-sketch/omc-doubao-oauth/actions/workflows/build.yml"><img src="https://github.com/paul19910215-sketch/omc-doubao-oauth/actions/workflows/build.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/CLIProxyAPI-v8+-4f46e5" alt="CLIProxyAPI v8+">
  <img src="https://img.shields.io/badge/license-MIT-blue" alt="MIT license">
</p>

<p align="center">
  简体中文 · <a href="README.md">English</a>
</p>

OMC Doubao OAuth 给 CLIProxyAPI 增加 `doubao` 提供商：用豆包网页/桌面账号扫码登录，然后在 CPA 里直接调用豆包对话模型。**原生支持 Anthropic Messages、OpenAI Chat Completions、OpenAI Responses 三种协议**，不需要本地浏览器，不需要豆包 CLI，也不会在 CPA 里多转一道格式。

本项目是独立第三方实现，与字节跳动无关联。请阅读 [NOTICE.md](NOTICE.md) 与文末免责声明。

## 原生协议

插件在执行器能力里声明了三种格式，CPA 会把对应请求直接交给插件，不再先做格式转换：

| 客户端端点 | CPA 格式 | 原生支持 | 说明 |
| --- | --- | --- | --- |
| `POST /v1/messages` | `claude` | 是 | `thinking` 块映射豆包深度思考 |
| `POST /v1/chat/completions` | `openai` | 是 | 思考内容以 `reasoning_content` 增量返回 |
| `POST /v1/responses` | `openai-response` | 是 | 支持 reasoning summary 与 `output_text` 事件 |

其他协议（Gemini、Codex 等）由 CPA 先转换成上面三种格式之一再进入插件。

## 工作原理

1. **扫码登录**：插件走豆包 passport 二维码流程
   （`/passport/safe/csrf_token` → `/passport/web/get_qrcode` →
   `/passport/web/check_qrconnect`），把 `sessionid`、`ttwid`、CSRF 等 Cookie
   写入 CPA 的 auth 文件。
2. **两种上游端点**：
   - `samantha`（默认）：`POST /samantha/chat/completion`，明文 JSON/SSE，仅靠
     Cookie 即可工作，可选 `msToken`。
   - `chat`：`POST /chat/completion`，桌面/CLI 主入口，使用 `content_block`
     请求体，通常需要字节风控签名（`a_bogus`）。使用它时把 `signer_url`
     指向本地签名服务。
3. **可替换签名器**：签名是一个独立接口。默认 `none` 直接发请求；`external`
   模式向本地 HTTP 服务索取额外请求头，因此可以接入基于真实浏览器的签名器，
   而不用改插件二进制。

## 环境要求

- CLIProxyAPI v8.0.51 及以上。
- 拥有豆包网页/桌面对话产品访问权限的账号。插件使用账号自身的登录态，
  **不需要**豆包 CLI（CLI 仅限团队/企业订阅）。
- 账号自身的额度与风控规则照常生效。

## 安装

先为你的平台构建动态库，放进 CPA 的插件目录：

```text
omc-doubao-oauth.dll    Windows
omc-doubao-oauth.dylib  macOS
omc-doubao-oauth.so     Linux
```

然后在 CPA 配置中启用：

```yaml
plugins:
  enabled: true
  dir: /path/to/plugins
  configs:
    omc-doubao-oauth:
      enabled: true
      endpoint: samantha
      signer: none
      max_concurrency: 3
```

在面板里打开插件的登录入口，用豆包 App 扫码并在手机上确认。凭据会写入 CPA
的 auth 目录。

## 配置

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `endpoint` | enum | `samantha` | `samantha`、`chat` |
| `signer` | enum | `none` | `none`、`external` |
| `signer_url` | string | 空 | 绝对 HTTP(S) 地址，`external` 时必填 |
| `bot_id` | string | `7234781073513644036` | 每次请求使用的豆包 bot id |
| `deep_think` | boolean | `false` | 强制每次调用都开启深度思考 |
| `max_concurrency` | integer | `3` | 每个凭据 1–100 并发 |
| `public_base_url` | string | 空 | 面板未传 base URL 时，用于拼接二维码图片地址 |
| `app_version` | string | `2.1.7` | 查询参数里的桌面客户端版本 |

## 签名器约定

`signer: external` 时，插件发送：

```http
POST <signer_url>
Content-Type: application/json

{"method":"POST","url":"https://www.doubao.com/chat/completion?...","headers":{"Cookie":["..."]},"body":"{...}"}
```

期望返回：

```json
{"headers":{"X-Bogus":"...","msToken":"..."}}
```

`Host`、`Content-Length`、`Connection`、`Cookie` 会被忽略，Cookie 始终由插件
自己维护。最可靠的方案是让签名器驱动一个真实浏览器页面，由页面自身的请求
钩子计算 `a_bogus`。细节见 [docs/USAGE.md](docs/USAGE.md)。

## 限制

- 当前版本仅处理文本。图片、文件、工具调用块会在提示词里标注，但不会执行。
- `chat` 端点通常需要签名；默认的 `samantha` 只需 Cookie。
- 豆包会话可能过期或触发风控：401 需要重新扫码，429 表示额度或频率受限。
- 这是逆向集成，豆包客户端更新后可能失效，也可能与厂商服务条款冲突。
  请只用自己的账号，并自行承担风险。

## 开发

```sh
cd go
go mod download
go test ./...
go vet ./...
```

构建原生插件（cgo 动态库，需要 C 编译器）：

```sh
make build          # 本机平台
make package        # 构建并打包
```

CI 会在打 tag 时为 Linux、macOS、Windows 构建发布包。

## 目录

```text
go/                 Go/cgo 插件实现
assets/             插件商店与 README 使用的 logo
docs/USAGE.md       使用说明、签名器约定、排障
.github/workflows/  测试、构建、tag 发布
```

## 许可证

MIT，见 [LICENSE](LICENSE)。

## 免责声明

本项目为非官方项目，与字节跳动无隶属、背书或赞助关系。"豆包""Doubao"
是其各自权利人的商标。使用账号登录态接入本插件时，请自行确认并遵守厂商
服务条款，自行承担由此产生的后果。

