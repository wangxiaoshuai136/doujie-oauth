<p align="center">
  <img src="assets/logo.png" width="96" height="96" alt="Doujie OAuth logo">
</p>

<h1 align="center">Doujie OAuth</h1>

<p align="center">
  为 <a href="https://github.com/router-for-me/CLIProxyAPI">CLIProxyAPI</a>
  提供豆姐国内版与 Dola 国际版的显式登录入口
</p>

<p align="center">
  <a href="https://github.com/wangxiaoshuai136/doujie-oauth/releases"><img src="https://img.shields.io/github/v/release/wangxiaoshuai136/doujie-oauth?label=release" alt="Release"></a>
  <a href="https://github.com/wangxiaoshuai136/doujie-oauth/actions/workflows/build.yml"><img src="https://github.com/wangxiaoshuai136/doujie-oauth/actions/workflows/build.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/CLIProxyAPI-v8.0.49%2B-4f46e5" alt="CLIProxyAPI v8.0.49+">
  <img src="https://img.shields.io/badge/license-MIT-blue" alt="MIT license">
</p>

<p align="center">
  <a href="README.md">English</a> · <a href="README.zh-CN.md">简体中文</a>
</p>

<p align="center">
  更新日志：<a href="CHANGELOG.md">CHANGELOG.md</a>
</p>

Doujie OAuth 为 CLIProxyAPI 增加 `doubao` provider。在面板中显式选择国内版或
Dola 国际版入口并完成登录后，CPA 即获得模型发现、Anthropic Messages、OpenAI Chat
Completions、OpenAI Responses 原生执行，以及标准化配额与重置卡能力——无需本地
常驻浏览器，也无需企业级命令行客户端。

> [!WARNING]
> 本项目仅供开源学习与技术交流，禁止商业或非法用途。项目与相关公司及商标无隶属、
> 认可或背书关系；使用本项目产生的一切后果由使用者自行承担。完整声明见文末。

## 两个登录入口，一个 provider

国内版与 Dola 国际版产生同一种凭证（`"type": "doubao"`）并共用同一个 executor，
但登录站点、请求参数和图标不同，因此注册为两个独立的 OAuth 入口，而不是根据 IP
或浏览器语言自动切换。

| 门户 | 登录入口 | 说明 |
| --- | --- | --- |
| 豆姐（国内版） | `doubao` | 默认国内入口：返回账号登录链接，不显示二维码；默认不自动打开浏览器 |
| 豆姐国际版（Dola） | `dola` | 默认国际入口：返回账号登录链接，不显示二维码；默认不自动打开浏览器 |

登录入口只显示国内版和国际版两个主入口。旧 provider ID 与桌面端导入仍保留内部兼容，
但不再显示为独立卡片，也不会生成二维码。

主入口不需要扫码。桌面端入口无需手机；若豆包客户端正在运行并占用 Cookies 数据库，
插件会返回明确的提示：从托盘完全退出豆包后，重新点击该入口即可。

国内和国际主入口会用系统默认 Chromium 浏览器打开独立登录窗口。你可以直接输入手机号、
密码或短信验证码；插件只通过本机回环 DevTools 读取登录后的会话，不保存账号密码。
`doubao-account` 保留为旧客户端兼容别名。

在支持逐入口元数据的面板（CPA v8.0.51+）上，每个入口各自成为一张卡片，带自己的
名称和图标。选入口即选站点；插件不会自动切换地区。

- `doubao` 与 `doubao-desktop` 生成国内版凭据。
- `dola` 生成国际版凭据，需要可用账号与相应网络环境。
- Dola 入口为可选功能，不影响现有国内账号、模型或配额数据。

## 面板无关

所有面向用户的部分都由宿主渲染：OAuth 卡片、配置字段、静态资源和配额视图都通过
CPA 的插件 ABI 与管理 API 呈现。插件在 CPA 自带的 Management Center 或任何对接
CPA 管理 API 的面板中表现一致。

OAuth 发起和轮询使用 CPA 原生受认证管理 API，公共插件资源仅提供静态 Logo 与可选
描述声明。插件不需要本地回调监听器，也不依赖独立浏览器进程。

## 功能

- 豆姐国内版与 Dola 国际版两个独立账号登录入口，不提供二维码，默认国内版，从不自动换区。
- Windows 桌面端会话导入，避免手机端与桌面端账号不一致。
- 桌面 Cookies 被占用时返回明确的退出客户端后重试提示。
- 默认浏览器账号登录，支持手机号、密码或短信验证码，无需扫码。
- 原生 Anthropic Messages、OpenAI Chat Completions、OpenAI Responses 执行。
- 深度思考与思维链流式回传。
- 真实 `5h` / `7d` 配额窗口、重置时间与额度重置卡核销。
- `samantha` 直连通道与可插拔外部签名器。
- 按凭证的并发控制。
- 全平台预编译发布包。

## 支持模型

默认模型 ID 带 `doujie/` 前缀（例如 `doujie/glm-5.3-flash`），用于和其他供应商的同名模型区分；下表省略前缀。前缀可通过 `model_prefix` 修改，设为 `none` 关闭。执行器仍兼容无前缀和旧 `doubao/` 前缀的请求。

实际可用模型以当前账号菜单的动态下发为准；下表是默认与兼容清单，服务端新增模型会自动出现。

| Model ID | 显示名称 | 上下文 / 输出上限 |
| --- | --- | --- |
| `doubao` | 豆姐 Auto | 256k / 未下发 |
| `doubao-seed-2.0-lite` | 豆姐 Seed 2.0 Lite | 256k / 未下发 |
| `doubao-seed-1.6` | 豆姐 Seed 1.6 | 256k / 未下发 |
| `doubao-seed-1.6-flash` | 豆姐 Seed 1.6 Flash | 256k / 未下发 |
| `doubao-pro` | 豆姐 Pro | 256k / 未下发 |
| `deepseek-v4.1-flash` | DeepSeek V4.1 Flash | 300k / 未下发 |
| `deepseek-v4-pro` | DeepSeek V4 Pro | 300k / 未下发 |
| `kimi-k3.1-pro` | Kimi K3.1 Pro | 300k / 未下发 |
| `kimi-k3.1-pro-high-speed` | Kimi K3.1 Pro 高速 | 300k / 未下发 |
| `kimi-k3` | Kimi K3 | 300k / 未下发 |
| `glm-5.3` | GLM 5.3 | 300k / 未下发 |
| `glm-5.3-flash` | GLM 5.3 Flash | 300k / 未下发 |
| `qwen4-max` | Qwen 4 Max | 300k / 未下发 |
| `qwen3-8-max` | Qwen 3.8 Max | 300k / 未下发 |
| `qwen3-8-flash` | Qwen 3.8 Flash | 300k / 未下发 |
| `doubao-video` | 豆姐视频（自动） | 8k / 2k |
| `seedance-2.5` | Seedance 2.5 | 8k / 2k |
| `seedance-2.0` | Seedance 2.0 | 8k / 2k |
| `seedance-2.0-fast` | Seedance 2.0 Fast | 8k / 2k |
| `seedance-2.0-mini` | Seedance 2.0 Mini | 8k / 2k |

模型标识包含 `think` 或 `reason` 时自动开启深度思考；也可用 `deep_think: true`
强制全局开启。

国内版文本模型会映射到豆包桌面客户端的模型菜单键：`doubao` → 自动（`9`），
`doubao-pro` → 豆包 2.1 Pro（`5`），`doubao-seed-2.0-lite` → 豆包 2.1 Lite
（`seed-lite-7b`）。这些请求会自动使用客户端的 `/chat/completion` 模型选择协议；
`endpoint: samantha` 只影响没有模型菜单映射的请求。

新版工作模式的第三方模型也使用同一套菜单协议，包括 DeepSeek、Kimi 和 GLM。
模型是否可见、价格倍率以及是否允许调用由豆包账号、套餐和服务端灰度决定；插件只负责
把请求映射到对应的 `model_item_key`，不会绕过上游的订阅或额度校验。
DeepSeek 和 GLM 的网关键带 `person-` 前缀，Kimi 使用 `kimi-` 前缀；第三方网关模型
统一使用 `item_id=10001`。
如果当前账号没有下发某个模型，插件会返回 `400 model_unavailable`，而不是把它误报成
上游故障；Dola 国际版只展示国内版兼容的 `doubao` 文本模型，避免把国内工作模式模型
错误暴露给不支持的账号。

### 视频模型

`doubao-video` / `seedance-*` 走豆包会话视频 agent。首次调用只返回生成方案；
用户确认后，在请求体加 `"confirm": true`，或在下一条用户消息中回复“确认生成”，
插件会提交一次视频任务并把最终 MP4 地址放在普通 chat 回复中。

```json
{
  "model": "doujie/seedance-2.0",
  "confirm": true,
  "messages": [{"role": "user", "content": "海边日落，镜头缓慢推进"}]
}
```

当前为文生视频。插件会先调用 `/samantha/skill/pack` 获取账号实际可用的模型，
校验请求模型后，提交网页操作栏使用的顶层 `chat_ability` 对象：

```json
{
  "ability_type": 17,
  "ability_param": "{\"ratio\":\"16:9\",\"model\":\"seedance_v2.0_mini\",\"duration\":10,\"resolution\":\"720p\",\"input_box_content\":{\"user_input_content\":\"...\",\"reply_message_format\":\"生成视频：%s\"}}"
}
```

`conversation_mode` 为 `1`，`mention_skill_list` / `mention_ext` 保持为空；不会静默切换到未请求的模型：

| 请求模型 | 官方模型值 | 说明 |
| --- | --- | --- |
| `doubao-video` | 账号默认模型 | 当前通常是 `seedance_v2.0_mini` |
| `seedance-2.0` | `seedance_v2.0_std` | 需要增强订阅；未开通会返回上游错误 |
| `seedance-2.0-fast` | `seedance_v2.0` | 通常无需增强订阅 |
| `seedance-2.0-mini` | `seedance_v2.0_mini` | 默认、通常无需增强订阅 |
| `seedance-2.5` | 服务端下发 | 仅当账号的实际 SkillPack 提供时可用 |

如果账号未开通 `Seedance 2.0`，请改用 Fast 或 Mini；这不是插件版本问题。
`query_video_gen_info.remain_count` 是聚合体验次数，不等于视频生成额度；最终是否
能生成仍由豆包服务端校验订阅和 7 天视频额度。

确认后的视频任务按凭证隔离：同一个 auth 文件同时只允许一个任务；提交前会检查
`has_generating_task`；遇到额度错误会解析服务端 `reset_at` 并写入该账号的冷却，
后续调用直接返回带 `Retry-After` 的 `429`，不再重复打豆包。可通过认证后的管理
接口查看或清除：

- `GET /v0/management/doujie-oauth/video/status`
- `POST /v0/management/doujie-oauth/video/cooldowns/clear`，请求体 `{"auth_id":""}`

这些接口不在公开插件资源路由下。

### 独立视频 Skill

配套的独立视频 Skill 已拆分到单独仓库：
[`wenbaolu6666-design/doubao-session-video`](https://github.com/wenbaolu6666-design/doubao-session-video)。
它把同一套“已登录豆包会话生成视频”的能力封装成独立 Python CLI，方便不能直接调用
CPA 插件的智能体使用。

## 安装

从 [Releases](https://github.com/wangxiaoshuai136/doujie-oauth/releases) 下载对应
平台的压缩包，将共享库放入 CPA 的插件目录，然后在 `config.yaml` 中启用：

```yaml
plugins:
  enabled: true
  dir: /path/to/plugins
  configs:
    doujie-oauth:
      enabled: true
      endpoint: samantha
      signer: none
      max_concurrency: 3
      runtime: auto
      # workspace: D:\WenPaul\Documents
      # sandbox_auth_type: ask_risky
      # public_base_url: https://cpa.example.com
```

商店与共享库名称为 `doujie-oauth`；CPA 凭证 provider 标识保持为 `doubao`。
登录、配置、配额与故障排查见 [docs/USAGE.md](docs/USAGE.md)。

推送 `v*` 标签时由 GitHub Actions 创建发布。

## 配置

| 字段 | 类型 | 默认值 | 可选值 |
| --- | --- | --- | --- |
| `endpoint` | 枚举 | `samantha` | `samantha`、`chat` |
| `signer` | 枚举 | `none` | `none`、`external` |
| `signer_url` | 字符串 | 空 | `external` 时必填的 HTTPS 地址，或回环地址上的 HTTP |
| `bot_id` | 字符串 | 国内默认 Bot | 留空或使用默认值时按入口选择站点 Bot |
| `deep_think` | 布尔 | `false` | 是否强制所有请求开启深度思考 |
| `max_concurrency` | 整数 | `3` | 每凭证 1–100 个并发请求 |
| `public_base_url` | 字符串 | 空 | 面板未传基础地址时用于拼接静态登录结果页地址 |
| `app_version` | 字符串 | `2.1.7` | 国内客户端版本标识 |
| `auto_open_browser` | 布尔 | `false` | 是否立即打开独立浏览器登录；默认 `false`，先返回账号登录链接，点开链接后再启动浏览器 |
| `browser` | 字符串 | `auto` | `auto`、`chrome`、`edge`，或浏览器可执行文件绝对路径 |
| `runtime` | 枚举 | `auto` | `auto`、`cloud`、`local`；仅影响工作任务 |
| `workspace` | 字符串 | 空 | `runtime: local` 使用的本地目录；`auto` 在设置后自动选本地 |
| `device_name` | 字符串 | 主机名 | 上报给豆包的本地设备名 |
| `sandbox_auth_type` | 枚举 | `ask_risky` | `ask_always`、`ask_risky`、`always_allow` |
| `local_app_id` | 字符串 | 聊天 app id | 覆盖本地任务使用的 app id |
| `client_os` | 枚举 | `Windows` | 本地桌面设备系统：`Windows`、`macOS`、`Linux` |
| `model_prefix` | 字符串 | `doujie/` | 暴露给客户端的模型 ID 前缀；设为 `none` 关闭 |

## 开发

插件实现位于 [`go/`](go/)。模块下载完成后，测试无需网络：

```sh
cd go
go mod download
GOPROXY=off GOSUMDB=off go test ./...
GOPROXY=off GOSUMDB=off go vet ./...
```

CI 从标签构建以下发布产物：

```text
doujie-oauth_<version>_linux_amd64.zip
doujie-oauth_<version>_linux_arm64.zip
doujie-oauth_<version>_darwin_amd64.zip
doujie-oauth_<version>_darwin_arm64.zip
doujie-oauth_<version>_windows_amd64.zip
checksums.txt
```

每个压缩包内含命名正确的 `doujie-oauth.so`、`doujie-oauth.dylib` 或
`doujie-oauth.dll`。本地原生构建使用 `make build`，`make package` 生成相同格式。

## 项目结构

```text
.github/workflows/   CI 测试、原生构建与标签发布
assets/logo.png      插件主 Logo
assets/doubao.png    国内版登录图标
assets/dola.png      国际版登录图标
go/                  Go/cgo 插件实现与测试
docs/USAGE.md        部署安装与使用指南
scripts/             Windows 桌面端导入脚本
```

## 许可证与免责声明

[MIT](LICENSE)

本项目仅供开源学习、协议研究与技术交流，禁止任何商业或非法用途。项目为独立第三方
实现，与相关公司及商标无隶属、合作、认可或背书关系。软件按“现状”提供，作者与贡献者
不对使用、测试或传播本项目的任何直接或间接后果承担责任，风险由使用者自行承担。
