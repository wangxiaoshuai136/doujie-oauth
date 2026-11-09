<p align="center">
  <img src="assets/logo.png" width="128" height="128" alt="OMC Doujie OAuth logo">
</p>

<h1 align="center">OMC Doujie OAuth</h1>

<p align="center">
  High-performance unofficial "Doujie" (豆姐) protocol gateway adapter plugin for
  <a href="https://github.com/router-for-me/CLIProxyAPI">CLIProxyAPI (CPA)</a>
</p>

<p align="center">
  <a href="https://github.com/wangxiaoshuai136/omc-doubao-oauth/actions/workflows/build.yml"><img src="https://github.com/wangxiaoshuai136/omc-doubao-oauth/actions/workflows/build.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/wangxiaoshuai136/omc-doubao-oauth/releases/latest"><img src="https://img.shields.io/github/v/release/wangxiaoshuai136/omc-doubao-oauth?color=blue&label=release" alt="Latest Release"></a>
  <img src="https://img.shields.io/badge/CLIProxyAPI-v8+-4f46e5" alt="CLIProxyAPI v8+">
  <img src="https://img.shields.io/badge/license-MIT-blue" alt="MIT license">
</p>

<p align="center">
  <a href="README.zh-CN.md">简体中文</a> · English
</p>

> ### ⚠️ Disclaimer & Terms of Use (Must Read)
>
> 1. **Educational & Research Purposes Only**: This project is an independent open-source technical experiment created strictly for academic research, network protocol study, multi-format API gateway conversion experiments, and personal learning. **Commercial use, production deployment, or unlawful activity is strictly prohibited.**
> 2. **Independent Third-Party Implementation**: Codenamed "**Doujie**" (豆姐), this software is an entirely independent protocol adapter. **It is not affiliated with, endorsed by, sponsored by, or associated with any commercial enterprise.** Any technical identifiers referenced are solely for illustrating message wire-formats; all relevant trademarks and copyrights belong exclusively to their respective owners.
> 3. **Provided AS-IS & Zero Liability**: This software and prebuilt binaries are provided "**AS IS**" without warranty of any kind, express or implied. **The authors and contributors assume NO liability or responsibility whatsoever for any direct or indirect consequences, losses, account restrictions, service suspensions, data corruption, or legal disputes resulting from the download, use, testing, or dissemination of this software. All risks are assumed entirely by the user.**
> 4. **Agreement by Use**: By viewing, downloading, cloning, or running this repository in any manner, you acknowledge that you have fully read, understood, and agreed to all terms in this notice without reservation. If you do not agree, discontinue use and delete all related files immediately.

---

## Overview

OMC Doujie OAuth extends CLIProxyAPI with multi-protocol gateway compatibility. Connect through QR pairing or import an existing local desktop client session to access compatible conversational large language models.

The plugin provides **native pass-through execution for Anthropic Messages, OpenAI Chat Completions, and OpenAI Responses**, featuring full real-time streaming for deep reasoning traces (CoT), quota window tracking, and quota reset-card redemption. It requires no persistent browser instance, no enterprise CLI tools, and incurs zero gateway protocol-translation overhead.

---

## Key Features

- ⚡ **Native Tri-Protocol Pass-Through**: Implements Claude, OpenAI, and OpenAI Response formats directly within the executor. CPA routes requests straight to the plugin, eliminating intermediate gateway translation hops and formatting loss.
- 🧠 **Deep-Think & Reasoning Streaming**: Seamlessly preserves Chain-of-Thought (CoT) traces. Claude `thinking` blocks, OpenAI `reasoning_content` deltas, and Responses reasoning summaries are delivered in real time.
- 🔑 **Dual Authentication Modes**:
  - **Mobile QR Sign-In (`doubao`)**: Generates a secure login QR code within the CPA management dashboard for mobile app authorization.
  - **Zero-Touch Local Session Import (`doubao-desktop`)**: Directly reads local desktop client session credentials without scanning. Eliminates account profile mismatches between mobile and PC logins. A standalone headless Python script is also included for server environments.
- 📊 **Usage Monitoring & Quota Reset Integration**:
  - Synchronizes account quota windows and reset-card inventory using authorized user sessions.
  - Accurately reports real `7d` and `5h` rolling quota buckets, actual usage numbers (e.g., `used 314 / 2100`), and second-precise reset timestamps.
  - Native support for **Quota Reset Cards**: track available card balance and redeem cards with one click from the CPA panel. Gracefully falls back to local request estimation for free-tier accounts.
- 🛡️ **Dual Channel Architecture**:
  - `samantha` (default): Lightweight direct channel operating with session credentials, requiring zero external signing dependencies.
  - `chat`: Secondary channel supporting pluggable external signer adapters (`external`) for advanced networking setups.
- 📦 **Automated Cross-Platform Binaries**: Precompiled shared libraries for Windows (amd64), Linux (amd64/arm64), and macOS (amd64/arm64) automatically packaged and released via GitHub Actions.

---

## Supported Models

The plugin registers corresponding models with intelligent reasoning detection:

| Model ID | Display Name | Context / Output Limit | Deep Think | Description |
| --- | --- | --- | --- | --- |
| `doubao` | 豆姐 Auto | 256k / 32k | Auto-detected | Default smart-routing model |
| `doubao-seed-2.0` | 豆姐 Seed 2.0 | 256k / 32k | Auto-detected | Flagship next-generation general model |
| `doubao-seed-2.0-lite` | 豆姐 Seed 2.0 Lite | 256k / 32k | Supported | Low-latency, lightweight variant |
| `doubao-seed-2.0-code` | 豆姐 Seed 2.0 Code | 256k / 32k | Supported | Specialized for programming & refactoring |
| `doubao-seed-1.6` | 豆姐 Seed 1.6 | 256k / 32k | Supported | Standard 1.6 generation model |
| `doubao-seed-1.6-thinking` | 豆姐 Seed 1.6 思考 | 256k / 32k | **Enabled by default** | Deep reasoning / CoT specialized model |
| `doubao-seed-1.6-flash` | 豆姐 Seed 1.6 Flash | 256k / 32k | Supported | High-throughput, fast-response variant |
| `doubao-pro` | 豆姐 Pro | 128k / 16k | Supported | Classic Pro model |

> **Tip**: Any model ID containing `think` or `reason` automatically enables deep thinking mode. You can also configure `deep_think: true` to enforce it globally.

---

## Native Protocol Mapping

| Client Endpoint | CPA Protocol | Native | Streaming & Reasoning Behavior |
| --- | --- | --- | --- |
| `POST /v1/messages` | `claude` | Yes | `thinking` blocks mapped to deep reasoning; content streamed via `content_block_delta` |
| `POST /v1/chat/completions` | `openai` | Yes | Reasoning streamed incrementally via `delta.reasoning_content` |
| `POST /v1/responses` | `openai-response` | Yes | Emits reasoning summary and `response.output_text.delta` events |

Other client protocols (Gemini, Codex, etc.) are converted by CPA into one of the three formats above before entering the plugin.

---

## Quick Start

### 1. Download Prebuilt Libraries

Download the archive matching your operating system and architecture from [GitHub Releases](https://github.com/wangxiaoshuai136/omc-doubao-oauth/releases/latest):

| OS Platform | Architecture | Archive File |
| --- | --- | --- |
| **Windows** | x86_64 (`amd64`) | `omc-doubao-oauth_0.1.0_windows_amd64.zip` (`omc-doubao-oauth.dll`) |
| **Linux** | x86_64 (`amd64`) | `omc-doubao-oauth_0.1.0_linux_amd64.zip` (`omc-doubao-oauth.so`) |
| **Linux** | ARM64 (`arm64`) | `omc-doubao-oauth_0.1.0_linux_arm64.zip` (`omc-doubao-oauth.so`) |
| **macOS** | Apple Silicon (`arm64`) | `omc-doubao-oauth_0.1.0_darwin_arm64.zip` (`omc-doubao-oauth.dylib`) |
| **macOS** | Intel (`amd64`) | `omc-doubao-oauth_0.1.0_darwin_amd64.zip` (`omc-doubao-oauth.dylib`) |

Extract the shared library into your CPA plugins directory (e.g., `plugins/omc-doubao-oauth.dll`).

### 2. Configure CPA

Add the plugin configuration block to your `config.yaml`:

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    omc-doubao-oauth:
      enabled: true
      endpoint: samantha       # Recommended: samantha (direct channel, zero signing setup)
      signer: none             # Signing mode: none or external
      max_concurrency: 3       # Concurrent requests per account (1-100)
      # public_base_url: http://127.0.0.1:8317  # Fallback CPA URL for building QR image paths
```

Start or restart your CPA core process.

---

## Authentication Guide

The plugin never switches regions automatically based on IP or browser language. Choose one of the two explicit QR entry points:

- **豆姐（国内版）**: domestic site and its corresponding login parameters.
- **豆姐国际版（Dola）**: `dola.com` with separate login parameters. Subject to upstream regional policy and normally requires an overseas network environment. This optional entry does not affect the default domestic account.

Windows users can additionally use the local client import:

### Option A: Local Client Session Import (Recommended)

If you have already signed in on your local desktop client:
1. Open the CPA management panel, navigate to OAuth / Sign-in providers, and click **豆姐（桌面端导入）**.
2. The plugin reads the local desktop client session and writes the credential instantly.
3. No mobile scanning required, completely avoiding profile mismatches between devices.

> You can also run the headless Python helper on servers:
> ```bash
> python scripts/import_doubao_desktop.py --auth-dir "/path/to/cpa/oauth"
> ```

### Option B: Mobile App QR Sign-In

1. In the CPA panel, select **豆姐（国内版）** or **豆姐国际版（Dola）** as needed.
2. A QR code for the selected site will be displayed (or accessed at `/v0/resource/plugins/omc-doubao-oauth/qr.png`).
3. Scan and confirm using the mobile app. The session is saved into CPA's auth store.

---

## Quota Tracking & Reset Cards

The plugin synchronizes subscription profiles to monitor quota windows:

- **Real-Time Display**: In the CPA management dashboard, the plugin shows genuine `7d` / `5h` buckets with actual usage numbers (e.g., `used 314 / 2100`) and the exact reset time.
- **Reset Card Action**: When reset cards are available, an **额度重置卡** card appears with a **Reset Quota** action button, allowing you to replenish your quota with one click.
- **Graceful Fallback**: For free accounts lacking server-side subscription windows, the plugin switches smoothly to local traffic estimation and labels it accordingly.

---

## Client Integration Examples

### 1. Cursor / Windsurf / Cline
- **API Base URL**: `http://127.0.0.1:8317/v1`
- **API Key**: Your CPA management token or client key
- **Model Name**: `doubao-seed-2.0` or `doubao-seed-1.6-thinking`

### 2. Claude Code / Anthropic SDK
```bash
export ANTHROPIC_BASE_URL="http://127.0.0.1:8317"
export ANTHROPIC_API_KEY="your-cpa-key"
```

### 3. OpenAI Python SDK
```python
from openai import OpenAI

client = OpenAI(
    base_url="http://127.0.0.1:8317/v1",
    api_key="your-cpa-key",
)

response = client.chat.completions.create(
    model="doubao-seed-1.6-thinking",
    messages=[{"role": "user", "content": "Which number is larger: 9.11 or 9.9? Explain step-by-step."}],
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

## Configuration Reference

| Field | Type | Default | Values | Description |
| --- | --- | --- | --- | --- |
| `endpoint` | string | `samantha` | `samantha`, `chat` | Connection channel: `samantha` (direct channel, recommended) or `chat` (secondary channel, supports external signer) |
| `signer` | string | `none` | `none`, `external` | Signing mode |
| `signer_url` | string | empty | absolute HTTP(S) URL | Required when `signer: external` (e.g., `http://127.0.0.1:8787/sign`) |
| `bot_id` | string | `7234781073513644036` | string | Target bot ID |
| `deep_think` | boolean | `false` | `true`, `false` | Force deep reasoning for every request |
| `max_concurrency` | integer | `3` | `1` to `100` | Max concurrent requests per credential |
| `public_base_url` | string | empty | absolute URL | External CPA URL for building panel QR code assets |
| `app_version` | string | `2.1.7` | string | Client version identifier |

---

## External Signer Adapter (`signer: external`)

When using `endpoint: chat`, the plugin can delegate extra header signing to `signer_url`:

Request sent by plugin:
```json
{
  "method": "POST",
  "url": "https://...",
  "headers": { "Cookie": ["..."] },
  "body": "{...}"
}
```

Expected signer response:
```json
{
  "headers": {
    "Custom-Sign-Header": "..."
  }
}
```

> Sensitive headers like `Host`, `Content-Length`, and `Cookie` are managed internally and ignored from the signer response. Keep the signer service bound to local loopback (`127.0.0.1`).

---

## Troubleshooting

| Symptom | Cause | Recommended Action |
| --- | --- | --- |
| `401 reauth_required` | Session expired or invalidated on other devices | Remove the expired auth file and sign in again (or re-import from desktop) |
| `429 quota_exhausted` | Quota exhausted or rate-limited | Check reset timestamp, or use the **Reset Quota** button to redeem a reset card |
| Free tier instead of subscriber | Mobile app logged into a different profile | Use **豆姐（桌面端导入）** to import the logged-in desktop profile directly |
| No 5h bucket in quota card | Policy difference per subscription plan | Some plans only provide 7d rolling windows; 5h buckets appear automatically when provided |
| `502 upstream_error` | Upstream network timeout or connection reset | Check internet connectivity; prefer the stable `endpoint: samantha` |
| QR image 404 | Flow timed out (5-minute TTL) or plugin restarted | Refresh the panel page and initiate sign-in again |

---

## Development & Building

The plugin is written in Go and cgo:

```bash
# Download dependencies and run test suite
cd go
go mod download
go test ./...
go vet ./...

# Build shared library for current OS (requires gcc/MinGW/clang)
make build

# Package release archives
make package
```

---

## Project Structure

```text
├── go/                     # Go/cgo plugin implementation
│   ├── auth.go             # QR flow and credential lifecycle
│   ├── desktop_import*.go  # Local desktop client session import
│   ├── doubao.go           # Core protocol transport
│   ├── formats.go          # Claude, OpenAI, and Responses serializers
│   ├── models.go           # Model definitions and deep-think detection
│   ├── quota_api.go        # Quota and reset-card handlers
│   └── signer.go           # Pluggable external signer bridge
├── assets/                 # Icons and static resources
├── docs/                   # Detailed usage guides and signer specs
├── scripts/                # Headless desktop import Python script
└── .github/workflows/      # Automated multi-platform CI/CD release workflow
```

---

## License & Legal Notice

- Distributed under the [MIT License](LICENSE).
- **Disclaimer**: This software is an independent open-source technical exploration. The authors and contributors provide no warranties and assume no liability for any direct or indirect consequences, losses, or legal disputes resulting from the use of this project. Users are solely responsible for ensuring compliance with all applicable laws and third-party terms.
