<p align="center">
  <img src="assets/logo.png" width="96" height="96" alt="OMC Doujie OAuth logo">
</p>

<h1 align="center">OMC Doujie OAuth</h1>

<p align="center">
  为 <a href="https://github.com/router-for-me/CLIProxyAPI">CLIProxyAPI (CPA)</a> 打造的高性能非官方「豆姐（Doujie）」Provider 插件
</p>

<p align="center">
  <a href="https://github.com/wangxiaoshuai136/omc-doubao-oauth/actions/workflows/build.yml"><img src="https://github.com/wangxiaoshuai136/omc-doubao-oauth/actions/workflows/build.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/wangxiaoshuai136/omc-doubao-oauth/releases/latest"><img src="https://img.shields.io/github/v/release/wangxiaoshuai136/omc-doubao-oauth?color=blue&label=release" alt="Latest Release"></a>
  <img src="https://img.shields.io/badge/CLIProxyAPI-v8+-4f46e5" alt="CLIProxyAPI v8+">
  <img src="https://img.shields.io/badge/license-MIT-blue" alt="MIT license">
</p>

<p align="center">
  简体中文 · <a href="README.md">English</a>
</p>

OMC Doujie OAuth 为 CLIProxyAPI 扩展了“豆姐”系列对话模型提供商能力。通过移动端扫码或本地桌面端会话无感导入，即可在 CPA 中无缝调用全系列对话模型。

插件**原生直通 Anthropic Messages、OpenAI Chat Completions 与 OpenAI Responses 三大主流协议**，支持思维链深度流式回传、官方实时订阅配额读取以及额度重置卡在线核销。全程无需本地常驻浏览器，无需开通企业级命令行工具，且避免了网关层多次协议转换可能带来的格式损耗。

> [!NOTE]
> 本项目为开源第三方实现，项目代号“豆姐”（Doujie），旨在划清商标界限并提供通用的多协议本地网关支持。本项目与北京字节跳动科技有限公司无任何隶属、认可或背书关系。请参阅 [NOTICE.md](NOTICE.md) 与文末免责声明。

---

## 核心特性

- ⚡ **原生三协议直通架构**：在插件执行器内完整声明并实现 Claude、OpenAI 与 OpenAI Response 格式，由 CPA 核心直派请求，杜绝网关层中间转换带来的信息截断或延迟。
- 🧠 **思维链与深度思考完整回传**：全面适配深度思考与推理能力（CoT）。Claude `thinking` 块、OpenAI `reasoning_content` 及 Responses `reasoning` 摘要均可平滑流式输出。
- 🔑 **双形态登录鉴权体系**：
  - **App 扫码登录 (`doubao`)**：CPA 面板直接渲染二维码，移动端扫码一键授权。
  - **Windows 桌面端无感导入 (`doubao-desktop`)**：一键自动读取本地已登录的桌面客户端会话，免除扫码步骤，彻底避免手机端与电脑端账号不一致的问题。另提供独立无头 Python 脚本支持服务器环境。
- 📊 **官方订阅配额与重置卡联动**：
  - 自动同步官方订阅额度与配额重置卡信息（基于已授权的用户会话安全直连，无需额外服务）。
  - 精准同步展示 `7d` / `5h` 真实滚动配额、已用量/总量（如 `314 / 2100`）及精准重置时间戳。
  - 深度支持「额度重置卡」：在 CPA 面板实时监控可用重置卡张数，并支持一键发起重置卡核销；普通免费体验账号自动优雅降级为本地请求统计估算。
- 🛡️ **双通道模式设计**：
  - `samantha` 通道（默认推荐）：轻量化直连通道，仅凭账号会话鉴权即可稳定工作，零外部服务依赖。
  - `chat` 通道：支持挂载本地外部签名适配器（`external`），用于高阶网络与环境适配。
- 📦 **全平台预编译二进制**：通过 GitHub Actions 自动化编译发布 Windows (amd64)、Linux (amd64/arm64) 与 macOS (amd64/arm64)，下载即用。

---

## 支持模型列表

插件注册了对应体系的主流模型，并支持智能思维链识别：

| 模型标识 (Model ID) | 显示名称 | 上下文 / 输出限制 | 深度思考 (Deep Think) | 特性说明 |
| --- | --- | --- | --- | --- |
| `doubao` | 豆姐 Auto | 256k / 32k | 自动适配 | 默认智能路由模型 |
| `doubao-seed-2.0` | 豆姐 Seed 2.0 | 256k / 32k | 自动适配 | 综合能力最强的主力通用模型 |
| `doubao-seed-2.0-lite` | 豆姐 Seed 2.0 Lite | 256k / 32k | 支持 | 低延迟、轻量化响应版本 |
| `doubao-seed-2.0-code` | 豆姐 Seed 2.0 Code | 256k / 32k | 支持 | 针对编程辅助与代码生成特化 |
| `doubao-seed-1.6` | 豆姐 Seed 1.6 | 256k / 32k | 支持 | 经典 1.6 代际标准模型 |
| `doubao-seed-1.6-thinking` | 豆姐 Seed 1.6 思考 | 256k / 32k | **默认开启** | 深度推理 / 思维链增强特化模型 |
| `doubao-seed-1.6-flash` | 豆姐 Seed 1.6 Flash | 256k / 32k | 支持 | 高吞吐、极速响应版本 |
| `doubao-pro` | 豆姐 Pro | 128k / 16k | 支持 | 经典 Pro 模型 |

> **提示**：当请求的模型标识中包含 `think` 或 `reason` 时，插件会自动启用深度推理。也可以在配置文件中指定 `deep_think: true` 强制全局开启。

---

## 原生协议与流式映射

插件向 CPA 声明支持以下三种原生协议，对应格式由插件自行解析与拼装响应：

| 客户端调用端点 | CPA 协议标识 | 原生支持 | 流式响应映射与思维链行为 |
| --- | --- | --- | --- |
| `POST /v1/messages` | `claude` | 是 | `thinking` 块对应深度思考痕迹，文本流走 `content_block_delta` |
| `POST /v1/chat/completions` | `openai` | 是 | 深度思考内容通过 `delta.reasoning_content` 增量输出 |
| `POST /v1/responses` | `openai-response` | 是 | 输出 reasoning summary 以及 `response.output_text.delta` 事件 |

对于其他客户端协议（如 Gemini、Codex 等），CPA 会在网关层自动适配至上述原生格式之一再进入插件。

---

## 快速上手

### 1. 下载预编译动态库

从 [GitHub Releases](https://github.com/wangxiaoshuai136/omc-doubao-oauth/releases/latest) 下载适用于你系统的压缩包：

| 系统平台 | 架构 | 文件名 |
| --- | --- | --- |
| **Windows** | x86_64 (`amd64`) | `omc-doubao-oauth_0.1.0_windows_amd64.zip` (`omc-doubao-oauth.dll`) |
| **Linux** | x86_64 (`amd64`) | `omc-doubao-oauth_0.1.0_linux_amd64.zip` (`omc-doubao-oauth.so`) |
| **Linux** | ARM64 (`arm64`) | `omc-doubao-oauth_0.1.0_linux_arm64.zip` (`omc-doubao-oauth.so`) |
| **macOS** | Apple Silicon (`arm64`) | `omc-doubao-oauth_0.1.0_darwin_arm64.zip` (`omc-doubao-oauth.dylib`) |
| **macOS** | Intel (`amd64`) | `omc-doubao-oauth_0.1.0_darwin_amd64.zip` (`omc-doubao-oauth.dylib`) |

解压后将动态库放入 CPA 的插件目录（如 `plugins/windows/amd64/omc-doubao-oauth-v0.1.0.dll` 或 `plugins/omc-doubao-oauth.dll`）。

### 2. 启用插件配置

在 CPA 的 `config.yaml` 中添加插件配置项：

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    omc-doubao-oauth:
      enabled: true
      endpoint: samantha       # 默认推荐 samantha（直连通道，零外部依赖）
      signer: none             # 签名模式：none（直连）或 external（外部签名器）
      max_concurrency: 3       # 单账号并发数控制（1~100）
      # public_base_url: http://127.0.0.1:8317  # 若面板未传递 Base URL，用于拼接二维码资源路径
```

保存后启动或重启 CPA 核心服务。

---

## 账号登录指南

插件提供两种认证入口，满足不同的使用场景：

### 方式 A：Windows 桌面端无感导入（推荐）

如果你的电脑上已安装并登录了官方 Windows 桌面客户端：
1. 打开 CPA 管理面板，在 OAuth / 插件登录列表中找到 **豆姐（桌面端导入）**。
2. 点击后，插件将直接读取本机桌面客户端已登录会话并自动写入凭据。
3. 导入秒级完成，无需手机扫码，彻底保证导入的账号与桌面端保持一致（包含已开通的会员/体验套餐与重置卡）。

> 也可以在无头服务器或自动化脚本中直接运行 Python 导入工具：
> ```bash
> python scripts/import_doubao_desktop.py --auth-dir "/path/to/cpa/oauth"
> ```

### 方式 B：App 扫码登录

1. 打开 CPA 管理面板，点击 **豆姐** 登录项。
2. 面板将呈现安全登录二维码（也可通过 `/v0/resource/plugins/omc-doubao-oauth/qr.png` 访问）。
3. 使用手机端 App 扫码并确认登录，插件自动捕获会话并持久化至 CPA 凭据库。

---

## 官方订阅配额与重置卡管理

插件与账号订阅状态深度集成，通过已授权的用户会话自动同步当前配额与权益信息：

- **实时配额监控**：在 CPA 额度管理面板中直观呈现 `7d` / `5h` 滚动配额桶，清晰展示当前周期的已用额度、配额总量（如 `已用 314 / 2100`）以及精准到秒的配额重置时间。
- **重置卡在线核销**：当账号持有可用的额度重置卡时，面板会自动展示「额度重置卡」卡片并开放 **Reset Quota（重置额度）** 操作。点击即可直接核销重置卡并实时刷新当前周期额度。
- **平滑降级保护**：当账号为纯免费普通账号（上游不返回固定订阅窗口数据）时，插件将自动平滑降级为基于本地请求计数的估算模式，保障面板数据展示的连续性。

---

## 客户端接入示例

通过 CPA 启动后，模型即可作为标准的 OpenAI 或 Claude 接口被任意第三方客户端调用：

### 1. Cursor / Windsurf / Cline
- **API Base URL**: `http://127.0.0.1:8317/v1`
- **API Key**: 你的 CPA 管理密钥（或自定义 Client Token）
- **Model Name**: `doubao-seed-2.0` 或 `doubao-seed-1.6-thinking`

### 2. Claude Code / Anthropic SDK
```bash
export ANTHROPIC_BASE_URL="http://127.0.0.1:8317"
export ANTHROPIC_API_KEY="your-cpa-key"
```

### 3. OpenAI Python SDK 调用示例
```python
from openai import OpenAI

client = OpenAI(
    base_url="http://127.0.0.1:8317/v1",
    api_key="your-cpa-key",
)

response = client.chat.completions.create(
    model="doubao-seed-1.6-thinking",
    messages=[{"role": "user", "content": "9.11 和 9.9 哪个数字更大？请详细说明推理过程。"}],
    stream=True,
)

for chunk in response:
    delta = chunk.choices[0].delta
    if hasattr(delta, "reasoning_content") and delta.reasoning_content:
        print(delta.reasoning_content, end="", flush=True)
    if delta.content:
        print(delta.content, end="", flush=True)
```

---

## 配置项详解

| 字段名 | 类型 | 默认值 | 可选值 | 说明 |
| --- | --- | --- | --- | --- |
| `endpoint` | string | `samantha` | `samantha`, `chat` | 连接通道：`samantha`（默认直连通道，零外部依赖）；`chat`（扩展通道，支持挂载外部签名服务） |
| `signer` | string | `none` | `none`, `external` | 签名策略：`none`（直接通信）；`external`（转发至本地签名服务） |
| `signer_url` | string | 空 | 完整 HTTP(S) 地址 | 当 `signer: external` 时必填，如 `http://127.0.0.1:8787/sign` |
| `bot_id` | string | `7234781073513644036` | 字符串 | 目标 Bot ID |
| `deep_think` | boolean | `false` | `true`, `false` | 是否强制所有模型调用均开启深度推理 |
| `max_concurrency` | integer | `3` | `1` ~ `100` | 单个认证凭据允许的最大请求并发数 |
| `public_base_url` | string | 空 | 完整 URL | CPA 外部访问地址，用于生成面板可访问的二维码图片地址 |
| `app_version` | string | `2.1.7` | 字符串 | 客户端标识版本号 |

---

## 进阶：外部签名适配器 (`signer: external`)

默认推荐使用 `endpoint: samantha`（无需任何外部签名服务）。若在特定高级场景下切换为 `endpoint: chat`，插件支持将请求转发给本地签名服务获取补充请求头：

插件向 `signer_url` 发送的请求格式：
```json
{
  "method": "POST",
  "url": "https://...",
  "headers": { "Cookie": ["..."] },
  "body": "{...}"
}
```

外部服务只需返回需要追加的请求头：
```json
{
  "headers": {
    "Custom-Sign-Header": "..."
  }
}
```

> **注意**：敏感头（如 `Host`、`Content-Length`、`Cookie` 等）始终由插件内部接管，外部服务返回的同名头会被自动忽略。建议外部签名服务仅绑定本地回环地址（`127.0.0.1`）。

---

## 常见问题与排障

| 现象 | 可能原因 | 推荐排查方案 |
| --- | --- | --- |
| `401 reauth_required` | 登录凭据过期或已在其他终端失效 | 删除失效的 auth 文件，重新扫码或通过桌面端重新导入 |
| `429 quota_exhausted` | 触发账号频控或本周期配额已耗尽 | 检查 7d/5h 额度重置时间；或在面板点击「Reset Quota」核销一张重置卡 |
| 扫码后发现不是会员号 | 手机端登录账号与电脑桌面端不一致 | 推荐改用 **豆姐（桌面端导入）**，直接无感同步桌面端的会员登录态 |
| 额度面板未显示 5h 桶 | 官方按不同订阅计划划分策略 | 部分会员或体验计划只提供 7 天滚动额度池，有 5h 策略时会自动显示 |
| `502 upstream_error` | 上游网络超时或连接中断 | 检查网络连通性；推荐首选稳定免签名的 `endpoint: samantha` |
| 二维码图片 404 | 登录流超时（有效期 5 分钟）或服务重启 | 刷新面板重新发起登录请求即可 |

---

## 本地开发与构建

插件使用 Go 与 cgo 构建共享动态链接库：

```bash
# 下载依赖与运行测试
cd go
go mod download
go test ./...
go vet ./...

# 编译当前系统平台的动态库（需本地具备 gcc/MinGW/clang 编译环境）
make build

# 打包构建发布产物
make package
```

---

## 目录结构

```text
├── go/                     # Go/cgo 插件核心源码
│   ├── auth.go             # 扫码与凭据生命周期管理
│   ├── desktop_import*.go  # 本地桌面客户端会话导入实现
│   ├── doubao.go           # 核心协议交互与请求处理
│   ├── formats.go          # Claude / OpenAI / Responses 原生格式序列化
│   ├── models.go           # 模型列表定义与思维链识别
│   ├── quota_api.go        # 订阅配额与重置卡管理
│   └── signer.go           # 可插拔外置签名服务调用
├── assets/                 # 插件图标与静态资源
├── docs/                   # 运维文档与签名器实现参考
├── scripts/                # 独立桌面端导入 Python 脚本
└── .github/workflows/      # 全平台自动化编译与 Release 流水线
```

---

## 许可证与免责声明

- 本项目基于 [MIT 许可证](LICENSE) 开源。
- **免责声明**：本项目为开源客户端适配工具，代号“豆姐（Doujie）”，旨在划清商标界限并提供通用的多协议本地网关支持。本项目与北京字节跳动科技有限公司无任何隶属、认可或背书关系。“豆包”与“Doubao”为相关权利人的注册商标。使用者应自行对账号使用行为及服务条款合规性负责，开发者不对因使用本工具可能引发的账户变动或服务限制承担责任。
