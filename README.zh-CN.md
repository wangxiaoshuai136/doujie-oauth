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
  <img src="https://img.shields.io/badge/CLIProxyAPI-v8+-4f46e5" alt="CLIProxyAPI v8+">
  <img src="https://img.shields.io/badge/license-MIT-blue" alt="MIT license">
</p>

<p align="center">
  <a href="README.md">English</a> · <a href="README.zh-CN.md">简体中文</a>
</p>

Doujie OAuth 为 CLIProxyAPI 增加 `doubao` provider。在面板中显式选择国内版或
Dola 国际版入口并完成扫码后，CPA 即获得模型发现、Anthropic Messages、OpenAI Chat
Completions、OpenAI Responses 原生执行，以及标准化配额与重置卡能力——无需本地
常驻浏览器，也无需企业级命令行客户端。

> [!WARNING]
> 本项目仅供开源学习与技术交流，禁止商业或非法用途。项目与相关公司及商标无隶属、
> 认可或背书关系；使用本项目产生的一切后果由使用者自行承担。完整声明见文末。

## 两个扫码门户，一个 provider

国内版与 Dola 国际版产生同一种凭证（`"type": "doubao"`）并共用同一个 executor，
但登录站点、请求参数和图标不同，因此注册为两个独立的 OAuth 入口，而不是根据 IP
或浏览器语言自动切换。

| 门户 | 登录入口 | 说明 |
| --- | --- | --- |
| 豆姐（国内版） | `doubao` | 默认国内站点，使用 `assets/doubao.png` |
| 豆姐国际版（Dola） | `dola` | 独立国际站点，使用 `assets/dola.png`；受上游地区策略影响 |
| 豆姐（桌面端导入） | `doubao-desktop` | Windows 本地会话无感导入，归入国内凭证 |

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

- 豆姐国内版与 Dola 国际版两个独立扫码入口，默认国内版，从不自动换区。
- Windows 桌面端会话导入，避免手机端与桌面端账号不一致。
- 原生 Anthropic Messages、OpenAI Chat Completions、OpenAI Responses 执行。
- 深度思考与思维链流式回传。
- 真实 `5h` / `7d` 配额窗口、重置时间与额度重置卡核销。
- `samantha` 直连通道与可插拔外部签名器。
- 按凭证的并发控制。
- 全平台预编译发布包。

## 支持模型

| Model ID | 显示名称 | 上下文 / 输出 |
| --- | --- | --- |
| `doubao` | 豆姐 Auto | 256k / 32k |
| `doubao-seed-2.0` | 豆姐 Seed 2.0 | 256k / 32k |
| `doubao-seed-2.0-lite` | 豆姐 Seed 2.0 Lite | 256k / 32k |
| `doubao-seed-2.0-code` | 豆姐 Seed 2.0 Code | 256k / 32k |
| `doubao-seed-1.6` | 豆姐 Seed 1.6 | 256k / 32k |
| `doubao-seed-1.6-thinking` | 豆姐 Seed 1.6 思考 | 256k / 32k |
| `doubao-seed-1.6-flash` | 豆姐 Seed 1.6 Flash | 256k / 32k |
| `doubao-pro` | 豆姐 Pro | 128k / 16k |

模型标识包含 `think` 或 `reason` 时自动开启深度思考；也可用 `deep_think: true`
强制全局开启。

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
| `signer_url` | 字符串 | 空 | `external` 时必填的绝对 HTTP(S) 地址 |
| `bot_id` | 字符串 | 国内默认 Bot | 留空或使用默认值时按入口选择站点 Bot |
| `deep_think` | 布尔 | `false` | 是否强制所有请求开启深度思考 |
| `max_concurrency` | 整数 | `3` | 每凭证 1–100 个并发请求 |
| `public_base_url` | 字符串 | 空 | 面板未传基础地址时用于拼接二维码资源地址 |
| `app_version` | 字符串 | `2.1.7` | 国内客户端版本标识 |

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
*.zip.sha256
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
