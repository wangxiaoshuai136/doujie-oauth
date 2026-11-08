<p align="center">
  <img src="assets/logo.png" width="96" height="96" alt="OMC Doubao OAuth logo">
</p>

<h1 align="center">OMC Doubao OAuth</h1>

<p align="center">
  Unofficial Doubao (豆包) provider for
  <a href="https://github.com/router-for-me/CLIProxyAPI">CLIProxyAPI</a>
</p>

<p align="center">
  <a href="https://github.com/paul19910215-sketch/omc-doubao-oauth/actions/workflows/build.yml"><img src="https://github.com/paul19910215-sketch/omc-doubao-oauth/actions/workflows/build.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/CLIProxyAPI-v8+-4f46e5" alt="CLIProxyAPI v8+">
  <img src="https://img.shields.io/badge/license-MIT-blue" alt="MIT license">
</p>

<p align="center">
  <a href="README.zh-CN.md">简体中文</a> · English
</p>

OMC Doubao OAuth adds the `doubao` provider to CLIProxyAPI. Sign in with a
Doubao web/desktop account by scanning a QR code, then use Doubao chat models
through CPA with **native Anthropic Messages, OpenAI Chat Completions and
OpenAI Responses support** — no local browser, no Doubao CLI, and no
format-conversion hop for those three protocols.

This is an independent, unofficial project. It is not affiliated with
ByteDance. See [NOTICE.md](NOTICE.md) and the disclaimer below.

## Native protocols

The plugin declares all three formats in its executor capability, so CPA hands
requests straight to the plugin instead of translating them first:

| Client endpoint | CPA format | Native | Notes |
| --- | --- | --- | --- |
| `POST /v1/messages` | `claude` | yes | `thinking` blocks are mapped to Doubao deep-think traces |
| `POST /v1/chat/completions` | `openai` | yes | reasoning is exposed as `reasoning_content` deltas |
| `POST /v1/responses` | `openai-response` | yes | reasoning summary + `output_text` events |

Other client protocols (Gemini, Codex, ...) are converted by CPA to one of the
three formats above before reaching the plugin.

## How it works

1. **QR sign-in.** The plugin starts Doubao's passport QR flow
   (`/passport/safe/csrf_token` → `/passport/web/get_qrcode` →
   `/passport/web/check_qrconnect`) and stores the resulting `sessionid`,
   `ttwid` and CSRF cookies in a CPA auth file.
2. **Two upstream endpoints.**
   - `samantha` (default): `POST /samantha/chat/completion`, a plain JSON/SSE
     endpoint. Works with session cookies and an optional `msToken`.
   - `chat`: `POST /chat/completion`, the desktop/CLI main entry. It uses
     `content_block` payloads and normally requires ByteDance request signing
     (`a_bogus`). Point `signer_url` at a local signer service when you use it.
3. **Pluggable signer.** The signing layer is an interface. The default
   `none` mode sends requests as-is. `external` mode asks a local HTTP service
   for the extra headers, so a browser-backed signer can be swapped in without
   touching the plugin binary.

## Requirements

- CLIProxyAPI v8.0.51 or newer.
- A Doubao account with access to the web/desktop chat product. The plugin uses
  your normal account session; it does not need the Doubao CLI, which is
  restricted to team/enterprise subscriptions.
- The account's own quota/risk rules still apply.

## Install

Build the shared library for your platform and place it in CPA's plugin
directory:

```text
omc-doubao-oauth.dll    Windows
omc-doubao-oauth.dylib  macOS
omc-doubao-oauth.so     Linux
```

Then enable it in CPA's configuration:

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

Open the plugin's sign-in tile in your panel, scan the QR code with the Doubao
mobile app, and confirm on the phone. Credentials are written to CPA's auth
directory.

## Configuration

| Field | Type | Default | Values |
| --- | --- | --- | --- |
| `endpoint` | enum | `samantha` | `samantha`, `chat` |
| `signer` | enum | `none` | `none`, `external` |
| `signer_url` | string | empty | absolute HTTP(S) URL, required for `external` |
| `bot_id` | string | `7234781073513644036` | Doubao bot id used for every request |
| `deep_think` | boolean | `false` | force deep thinking for every call |
| `max_concurrency` | integer | `3` | 1–100 requests per credential |
| `public_base_url` | string | empty | absolute CPA base URL used to render the QR image when the panel supplies no base URL |
| `app_version` | string | `2.1.7` | desktop client version sent in query parameters |

## Signer contract

With `signer: external`, the plugin sends:

```http
POST <signer_url>
Content-Type: application/json

{"method":"POST","url":"https://www.doubao.com/chat/completion?...","headers":{"Cookie":["..."]},"body":"{...}"}
```

and expects:

```json
{"headers":{"X-Bogus":"...","msToken":"..."}}
```

Headers named `Host`, `Content-Length`, `Connection` and `Cookie` are ignored;
the plugin keeps its own cookies. A signer that drives a real browser page and
lets the page's own request hook compute `a_bogus` is the most reliable option.
See [docs/USAGE.md](docs/USAGE.md).

## Quota display

The plugin reads Doubao's own account data instead of guessing:

| Endpoint | Purpose |
| --- | --- |
| `POST /alice/commerce/sale/subscription/quota/summary/` | subscription windows (`window_limit_section.window_limit_groups[].window_limits[]` with `used_amount` / `total_amount` / `start_time` / `end_time`) |
| `POST /alice/commerce/marketing/card/balance/` | reset-card stock (`pc_quota_reset_card`, `quota_reset_card`) |
| `POST /alice/commerce/marketing/card/use/` | consume one reset card |

The panel therefore shows real `5h` / `7d` buckets with reset times, plus an
**额度重置卡** card with the available card count and a reset action. When the
account has no subscription windows (free account) the plugin falls back to a
local request-based estimate and labels it as such.

If the QR sign-in attached a different account than the one you use in the
desktop client, import the desktop session instead:

```bash
python scripts/import_doubao_desktop.py --auth-dir <cpa-auth-dir>
```

The helper decrypts the desktop cookie store (DPAPI + AES-GCM) and writes a
`doubao` credential for the account the desktop client is signed into.

These commerce endpoints accept the session cookies directly and do **not**
require the chat request signature. Chat requests still need the `a_bogus`
signature on the `chat` endpoint; `samantha` works with cookies alone.
## Limitations

- Text-only in this version. Image, file and tool-use blocks are noted in the
  prompt but not executed.
- The `chat` endpoint usually needs signing; `samantha` is the default because
  it works with cookies alone.
- Doubao sessions can expire or be challenged by risk control. A 401 asks you
  to sign in again; a 429 means the account's quota or rate limit was hit.
- This is a reverse-engineered integration. It can break when Doubao changes
  its client, and it may conflict with the vendor's terms of service. Use it
  with your own account and at your own risk.

## Development

```sh
cd go
go mod download
go test ./...
go vet ./...
```

Build a native plugin (requires a C compiler because the plugin is a cgo
shared library):

```sh
make build          # native platform
make package        # build + zip
```

CI builds release archives for Linux, macOS and Windows on tags.

## Project layout

```text
go/                 Go/cgo plugin implementation
assets/             logo used by the plugin store and README
docs/USAGE.md       operator guide, signer contract, troubleshooting
.github/workflows/  tests, builds and tagged releases
```

## License

MIT. See [LICENSE](LICENSE).

## Disclaimer

This project is unofficial and is not affiliated with, endorsed by, or
sponsored by ByteDance. "Doubao" and "豆包" are trademarks of their respective
owners. You are responsible for complying with the vendor's terms and for any
consequences of using your account session through this plugin.





