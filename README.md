<p align="center">
  <img src="assets/logo.png" width="96" height="96" alt="OMC Doujie OAuth logo">
</p>

<h1 align="center">OMC Doujie OAuth</h1>

<p align="center">
  <a href="https://github.com/WizisCool/oh-my-cpa">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/WizisCool/oh-my-cpa/main/web/src/assets/brand/omc-wordmark-dark.svg">
      <img src="https://raw.githubusercontent.com/WizisCool/oh-my-cpa/main/web/src/assets/brand/omc-wordmark-light.svg" alt="Oh-My-CPA" height="22">
    </picture>
  </a>
</p>

<p align="center">
  Explicit Doujie domestic and Dola international sign-in entries for
  <a href="https://github.com/router-for-me/CLIProxyAPI">CLIProxyAPI</a>
</p>

<p align="center">
  <a href="https://github.com/wangxiaoshuai136/omc-doubao-oauth/releases"><img src="https://img.shields.io/github/v/release/wangxiaoshuai136/omc-doubao-oauth?label=release" alt="Release"></a>
  <a href="https://github.com/wangxiaoshuai136/omc-doubao-oauth/actions/workflows/build.yml"><img src="https://github.com/wangxiaoshuai136/omc-doubao-oauth/actions/workflows/build.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/CLIProxyAPI-v8+-4f46e5" alt="CLIProxyAPI v8+">
  <img src="https://img.shields.io/badge/license-MIT-blue" alt="MIT license">
</p>

<p align="center">
  <a href="README.md">English</a> · <a href="README.zh-CN.md">简体中文</a>
</p>

OMC Doujie OAuth adds the `doubao` provider to CLIProxyAPI. After you explicitly
choose the domestic or Dola international entry and finish QR sign-in, CPA gains
model discovery, native Anthropic Messages, OpenAI Chat Completions and OpenAI
Responses execution, plus normalized quota and reset-card support — no local
browser process and no enterprise CLI client.

> [!WARNING]
> This project is for open-source learning and technical exchange only. Commercial
> or unlawful use is prohibited. It is not affiliated with any company or
> trademark holder; all consequences of use are the user's responsibility. See the
> full disclaimer below.

## Two QR portals, one provider

The domestic and Dola international portals produce the same credential
(`"type": "doubao"`) and share one executor, but they use different sites,
request parameters and icons. They are registered as two separate OAuth entries
instead of switching regions automatically by IP or browser language.

| Portal | Entry | Notes |
| --- | --- | --- |
| Doujie (domestic) | `doubao` | Default domestic site; uses `assets/doubao.png` |
| Doujie international (Dola) | `dola` | Independent international site; uses `assets/dola.png`; subject to upstream regional policy |
| Doujie desktop import | `doubao-desktop` | Windows local session import; produces a domestic credential |

On panels that render per-entry metadata (CPA v8.0.51+), each entry appears as
its own card with its own name and icon. Picking an entry picks the site; the
plugin never switches regions automatically.

- `doubao` and `doubao-desktop` produce domestic credentials.
- `dola` produces an international credential and requires a usable account and
  matching network environment.
- The Dola entry is optional and does not affect existing domestic accounts,
  models or quota data.

## Panel-agnostic by design

Everything user-facing is rendered by the host: OAuth tiles, config fields,
static assets and quota views all go through CPA's plugin ABI and management
API. The plugin is first-class in
[Oh-My-CPA](https://github.com/WizisCool/oh-my-cpa) and works the same in CPA's
own Management Center or any other panel that speaks CPA's management API.

OAuth start and polling use CPA's authenticated management API. Public plugin
resources contain only static logos and the optional description declaration.
The plugin needs no local callback listener and no separate browser process.

## Features

- Separate domestic and Dola international QR entries; domestic by default, never
  auto-switching regions.
- Windows desktop session import to avoid phone/desktop account mismatch.
- Native Anthropic Messages, OpenAI Chat Completions and OpenAI Responses
  execution.
- Deep-thinking and reasoning streaming.
- Real `5h` / `7d` quota windows, reset times and reset-card redemption.
- `samantha` direct channel and pluggable external signer.
- Per-credential concurrency control.
- Cross-platform prebuilt release archives.

## Supported models

| Model ID | Display name | Context / output |
| --- | --- | --- |
| `doubao` | 豆姐 Auto | 256k / 32k |
| `doubao-seed-2.0` | 豆姐 Seed 2.0 | 256k / 32k |
| `doubao-seed-2.0-lite` | 豆姐 Seed 2.0 Lite | 256k / 32k |
| `doubao-seed-2.0-code` | 豆姐 Seed 2.0 Code | 256k / 32k |
| `doubao-seed-1.6` | 豆姐 Seed 1.6 | 256k / 32k |
| `doubao-seed-1.6-thinking` | 豆姐 Seed 1.6 思考 | 256k / 32k |
| `doubao-seed-1.6-flash` | 豆姐 Seed 1.6 Flash | 256k / 32k |
| `doubao-pro` | 豆姐 Pro | 128k / 16k |

Model IDs containing `think` or `reason` enable deep thinking automatically;
`deep_think: true` forces it globally.

## Installation

Download the archive for your platform from
[Releases](https://github.com/wangxiaoshuai136/omc-doubao-oauth/releases), place
the shared library in CPA's plugin directory, then enable it in `config.yaml`:

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
      # public_base_url: https://cpa.example.com
```

The store and shared-library name is `omc-doubao-oauth`; the CPA credential
provider identifier remains `doubao`. See
[docs/USAGE.md](docs/USAGE.md) for login, configuration, quota and
troubleshooting.

Releases are created by GitHub Actions whenever a `v*` tag is pushed.

## Configuration

| Field | Type | Default | Values |
| --- | --- | --- | --- |
| `endpoint` | enum | `samantha` | `samantha`, `chat` |
| `signer` | enum | `none` | `none`, `external` |
| `signer_url` | string | empty | Absolute HTTP(S) URL required by `external` |
| `bot_id` | string | Domestic default bot | Leave default to select the site bot by entry |
| `deep_think` | boolean | `false` | Force deep thinking for every request |
| `max_concurrency` | integer | `3` | 1–100 requests per credential |
| `public_base_url` | string | empty | Fallback CPA URL for QR image rendering |
| `app_version` | string | `2.1.7` | Domestic client version identifier |

## Development

The plugin implementation lives in [`go/`](go/). Tests run without network
access after modules are downloaded:

```sh
cd go
go mod download
GOPROXY=off GOSUMDB=off go test ./...
GOPROXY=off GOSUMDB=off go vet ./...
```

CI builds these release artifacts from a tag:

```text
omc-doubao-oauth_<version>_linux_amd64.zip
omc-doubao-oauth_<version>_linux_arm64.zip
omc-doubao-oauth_<version>_darwin_amd64.zip
omc-doubao-oauth_<version>_darwin_arm64.zip
omc-doubao-oauth_<version>_windows_amd64.zip
*.zip.sha256
```

Each archive contains the correctly named `omc-doubao-oauth.so`,
`omc-doubao-oauth.dylib` or `omc-doubao-oauth.dll`. Local native builds use
`make build`; `make package` creates the same archive format.

## Project layout

```text
.github/workflows/   CI tests, native builds and tagged releases
assets/logo.png      Main plugin logo
assets/doubao.png    Domestic sign-in icon
assets/dola.png      International sign-in icon
go/                  Go/cgo plugin implementation and tests
docs/USAGE.md        Operator installation and usage guide
scripts/             Windows desktop import helper
```

## License and disclaimer

[MIT](LICENSE)

This project is for open-source learning, protocol research and technical
exchange only. Commercial or unlawful use is prohibited. It is an independent
third-party implementation and is not affiliated with, endorsed by or sponsored
by any company or trademark holder. The software is provided "AS IS"; the
authors and contributors accept no liability for any direct or indirect
consequences of using, testing or distributing it. All risk remains with the
user.
