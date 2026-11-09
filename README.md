<p align="center">
  <img src="assets/logo.png" width="96" height="96" alt="Doujie OAuth logo">
</p>

<h1 align="center">Doujie OAuth</h1>

<p align="center">
  Explicit Doujie domestic and Dola international sign-in entries for
  <a href="https://github.com/router-for-me/CLIProxyAPI">CLIProxyAPI</a>
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

Doujie OAuth adds the `doubao` provider to CLIProxyAPI. After you explicitly
choose a domestic or Dola international entry and finish sign-in, CPA gains
model discovery, native Anthropic Messages, OpenAI Chat Completions and OpenAI
Responses execution, plus normalized quota and reset-card support — no local
browser process and no enterprise CLI client.

> [!WARNING]
> This project is for open-source learning and technical exchange only. Commercial
> or unlawful use is prohibited. It is not affiliated with any company or
> trademark holder; all consequences of use are the user's responsibility. See the
> full disclaimer below.

## Two sign-in portals, one provider

The domestic and Dola international portals produce the same credential
(`"type": "doubao"`) and share one executor, but they use different sites,
request parameters and icons. They are registered as two separate OAuth entries
instead of switching regions automatically by IP or browser language.

| Portal | Entry | Notes |
| --- | --- | --- |
| Doujie (domestic) | `doubao` | Default domestic site; uses `assets/doubao.png` |
| Doujie international (Dola) | `dola` | Independent international site; uses `assets/dola.png`; subject to upstream regional policy |
| Doujie desktop import | `doubao-desktop` | Windows local session import; produces a domestic credential |
| Doujie account sign-in | `doubao-account` | Opens an isolated Edge/Chrome window for phone, password or SMS login; no QR code |

QR scanning is optional. The desktop entry imports the local session without a
phone. If Doubao is running and holds its cookie database open, the plugin opens
a no-QR import page: quit Doubao from the tray, then retry on that page.

The account entry opens a real Doubao login page in an isolated browser profile.
Sign in there with the phone number, password or SMS code you already use; the
plugin reads the resulting session through a loopback-only DevTools connection.
It never stores the account password.

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
API. The plugin works the same in CPA's
own Management Center or any other panel that speaks CPA's management API.

OAuth start and polling use CPA's authenticated management API. Public plugin
resources contain only static logos and the optional description declaration.
The plugin needs no local callback listener and no separate browser process.

## Features

- Separate domestic and Dola international QR entries; domestic by default, never
  auto-switching regions.
- Windows desktop session import to avoid phone/desktop account mismatch.
- No-QR fallback page for a locked desktop cookie database.
- Browser-based account sign-in for phone, password or SMS login without scanning.
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
| `doubao-video` | 豆姐视频（自动） | 8k / 2k |
| `seedance-2.5` | Seedance 2.5 | 8k / 2k |
| `seedance-2.0` | Seedance 2.0 | 8k / 2k |
| `seedance-2.0-fast` | Seedance 2.0 Fast | 8k / 2k |
| `seedance-2.0-mini` | Seedance 2.0 Mini | 8k / 2k |

Model IDs containing `think` or `reason` enable deep thinking automatically;
`deep_think: true` forces it globally.

### Video models

`doubao-video` and `seedance-*` use the signed-in Doubao video agent. The first
chat request returns a short plan only. After the user confirms, set
`"confirm": true` (or send a second user message such as `确认生成`); the plugin
then submits one video task and returns the final MP4 URL in the normal chat
response.

```json
{
  "model": "seedance-2.0",
  "confirm": true,
  "messages": [{"role": "user", "content": "A sunset over the sea, slow dolly in."}]
}
```

The current flow is text-to-video. Model choice is passed to the Doubao agent as
a prompt-level preference; duration and resolution depend on the account's
available model capabilities.

### Companion video skill

The companion Python skill lives in its own repository:
[`wenbaolu6666-design/doubao-session-video`](https://github.com/wenbaolu6666-design/doubao-session-video).
It exposes the same signed-in-session video flow as a standalone CLI for agents
that cannot call the CPA plugin directly.

## Installation

Download the archive for your platform from
[Releases](https://github.com/wangxiaoshuai136/doujie-oauth/releases), place
the shared library in CPA's plugin directory, then enable it in `config.yaml`:

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

The store and shared-library name is `doujie-oauth`; the CPA credential
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
doujie-oauth_<version>_linux_amd64.zip
doujie-oauth_<version>_linux_arm64.zip
doujie-oauth_<version>_darwin_amd64.zip
doujie-oauth_<version>_darwin_arm64.zip
doujie-oauth_<version>_windows_amd64.zip
checksums.txt
```

Each archive contains the correctly named `doujie-oauth.so`,
`doujie-oauth.dylib` or `doujie-oauth.dll`. Local native builds use
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
