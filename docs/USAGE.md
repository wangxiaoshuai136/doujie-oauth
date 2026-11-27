# Operator Guide

## 1. Install the Library

Build or download `doujie-oauth.{dll,dylib,so}` and place it in the directory
CPA scans for plugins (`plugins.dir`). Enable the plugin in `config.yaml`:

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
      # client_os: Windows
      # model_prefix: doujie/
      # public_base_url: https://cpa.example.com
```

`public_base_url` is needed for remote panels or reverse proxies. Local panels
default to `http://127.0.0.1:<port>`; the plugin never appends resource paths to
CPA's OAuth callback URL.

`runtime` controls the Doubao work-mode execution environment. `cloud` keeps
the previous cloud-computer behavior; `local` creates a local-computer task
using `workspace`, the imported desktop device id, and the configured
`sandbox_auth_type`; `auto` picks local when `workspace` is set and cloud
otherwise. Ordinary chat models do not create work-mode tasks at all.

`model_prefix` controls the model IDs advertised by `/v1/models`; it defaults to
`doujie/`. Set it to `none` to disable the prefix. Requests using unprefixed IDs
or the legacy `doubao/` prefix are still accepted.

## 2. Authentication

### Option A: Desktop Session Import (Recommended)

1. Open the panel's plugin / OAuth page and choose **豆姐（桌面端导入）**.
2. The plugin reads the local Windows desktop client session and writes an auth
   credential immediately.
3. No mobile scanning required, ensuring the imported account matches your
   active desktop profile.

Doubao keeps its cookie database open while running. If the session is locked,
the import fails with a clear message: quit Doubao completely from the
notification-area tray, then start the same entry again. No unauthenticated
retry page is exposed.

Headless server import is also supported:

```bash
python scripts/import_doubao_desktop.py --auth-dir "path/to/cpa/oauth"
```

### Option B: Legacy Provider IDs

The login picker intentionally exposes only the primary `doubao` and `dola`
entries. The legacy `doubao-qr`, `dola-qr`, and `doubao-desktop` provider IDs are
still accepted when an older host targets them directly, but they are not shown
as separate cards.

The plugin does not auto-detect regions. Choose either **豆姐（国内版）** or **豆姐国际版（Dola）** explicitly in the panel. The international entry uses `dola.com` and is subject to the upstream regional policy; it is optional and does not change the default domestic account.

### Option C: Browser Account Sign-In (No QR)

Choose **豆姐（国内版）** or **豆姐国际版（Dola）**. The plugin opens an installed
Chromium browser (Chrome, Edge, Brave, Chromium or Vivaldi) in an isolated profile
and the matching login page.
Enter the phone number, password or SMS code there. A loopback-only DevTools
connection reads the resulting cookies; the plugin never receives or stores the
password. The browser and temporary profile are closed after success or expiry.

This mode requires a desktop Windows or macOS session and an installed
Chromium-compatible browser (Safari is not supported). `auto` probes the standard
Windows install locations on Windows and `/Applications` plus `~/Applications` on
macOS. Set `DOUJIE_BROWSER_PATH` if the browser is installed elsewhere. Linux
builds return a clear unsupported-platform error and do not fall back to QR.

Browser launch follows the plugin/software settings:

- `auto_open_browser` defaults to `false`: a normal login returns a manual account
  sign-in link without launching a browser. Set it to `true` to launch immediately.
- An explicit OAuth metadata `browser` value (for example `chrome` or `edge`) enables
  immediate browser sign-in; `auto_open=false` or `browser=none` returns the manual link.
- The primary and legacy login entries never return a QR code. Opening the manual link
  starts the isolated browser and asks for a phone number, password or SMS code.
- `browser: auto|chrome|edge|<absolute path>` selects the browser to launch.
  OAuth metadata `browser`, `browser_name`, `browser_path`, or `browser_executable`
  takes precedence when the host supplies it.
- The plugin still validates that the selected browser is Chromium-compatible and
  uses an isolated temporary profile for cookie capture.

### Session Lifetime

Sessions are long-lived. When upstream rejects credentials (HTTP 401),
delete the auth file and sign in or import again.

### Account Grouping

Every saved credential carries an `email` label and an `oauth_provider` entry
(`doubao` or `dola`). The management panel groups saved accounts by that entry,
so the domestic and Dola cards list only the accounts created through them even
though both credentials use `type: doubao`. Desktop imports are stored under the
domestic entry as well, with `legacy_provider: doubao-desktop` kept for
backward-compatible parsing.
## 3. Upstream Channel Modes

- **`samantha` (default, recommended)**: Lightweight direct channel operating
  strictly with session credentials. Zero external signing dependencies.
- **`chat`**: Secondary channel supporting request signature delegation via
  `signer: external`.

## 4. Signer Service Contract

When `signer: external` is configured, the plugin forwards request context:

```json
{
  "method": "POST",
  "url": "https://...",
  "headers": {"Content-Type": ["application/json"], "Accept": ["*/*"]},
  "body": "{...}"
}
```

It expects:

```json
{
  "headers": {"Custom-Sign-Header": "..."}
}
```

Rules:
- The service must answer within the host HTTP timeout.
- Session cookies, authorization headers and CSRF tokens are never sent to the
  signer. Only `Content-Type`, `Accept`, `User-Agent` and `Agw-Js-Conv` are
  forwarded.
- Cookie-derived query parameters such as `msToken`, `sessionid`, `sid_*` and
  passport CSRF tokens are stripped from the URL before it is sent to the signer
  and before the upstream request. The Cookie header remains local to the plugin.
- The full request body is still forwarded to the signer and may contain private
  prompts. Only configure a signer you trust; use `signer: none` otherwise.
- `signer_url` must use HTTPS, or plain HTTP only when the host is loopback
  (`127.0.0.1`, `::1`, `localhost`).
- `Host`, `Content-Length`, `Connection` and `Cookie` are managed by the plugin
  and ignored from signer responses.

## 5. Protocol Mapping

| Client | Request fields read | Response fields produced |
| --- | --- | --- |
| Anthropic Messages | `system`, `messages[].content`, `thinking` | `message_start` / `thinking_delta` / `text_delta` / `message_stop` |
| Chat Completions | `messages[].content`, `reasoning_effort` | `chat.completion.chunk` with `content` and `reasoning_content` |
| Responses | `instructions`, `input`, `reasoning.effort` | `response.output_text.delta`, `response.completed` |

Token counts are estimated where upstream streaming chunks do not supply usage totals.

## 5.1 Video Models

The plugin advertises `doujie/doubao-video`, `doujie/seedance-2.5`,
`doujie/seedance-2.0`, `doujie/seedance-2.0-fast`, and
`doujie/seedance-2.0-mini` under the default prefix; unprefixed IDs remain
accepted. Use a normal Chat Completions
request. The first request returns a short plan; send `"confirm": true` (or a
follow-up user message such as `确认生成`) to submit exactly one video task. The
final MP4 URL is returned in the assistant message.

Video generation uses the same top-level `chat_ability` object as the web
action bar (`ability_type: 17`). The plugin resolves the account's real model
options from `/samantha/skill/pack`, validates the requested model, and submits
`conversation_mode: 1` with an empty `mention_skill_list`. `Seedance 2.0`
(`seedance_v2.0_std`) requires an enhanced subscription; use Fast or Mini if the
account does not have it. `query_video_gen_info.remain_count` is not the video
quota; the server still checks subscription and the 7-day video quota. It is
currently text-to-video only.

Each credential has an isolated video-task lease: a second confirmed task for the
same auth returns `409 video_busy`, and a pre-submit `has_generating_task` check
prevents duplicate work. Quota responses are converted to a per-account cooldown
using the server's `reset_at`; later calls return `429` with `Retry-After`
instead of repeating the upstream request. Operators can view or clear this state
through authenticated management routes:

- `GET /v0/management/doujie-oauth/video/status`
- `POST /v0/management/doujie-oauth/video/cooldowns/clear` with `{"auth_id":""}`

These routes are not exposed under the public resource prefix.

## 6. Quota Card and Reset Cards

The plugin synchronizes subscription quotas and reset cards using authorized
user sessions:

- Displays active `7d` and `5h` rolling limit windows with current usage numbers
  and reset timestamps.
- Displays available quota reset-card inventory.
- Provides a **Reset Quota** action to redeem a card and refresh quota immediately.
- Falls back to local request-count estimation for free-tier accounts lacking
  server-side subscription windows.

## 6.1 Daily Credit Status Check

`scripts/daily_doubao_credit.ps1` reads the daily App/desktop credit tasks for
every `doubao*.json` credential under the configured auth directory. It does not
start or require a local desktop client and writes an owner-local daily log under
`outputs/daily-credit/` without printing cookies.

The App task still requires a real mobile-app login. The desktop task status is
reported as returned by the server; the script does not fabricate a client login.

## 7. Troubleshooting

| Symptom | Meaning | Action |
| --- | --- | --- |
| `401 reauth_required` | Session expired or challenged | Delete the auth file, sign in or re-import |
| `429 verification_required` | Doubao risk-control verification challenge (`710022004`) | Complete the verification in Doubao, then retry after `retry_after_seconds` |
| `429 rate_limited` | Short-term account rate limit (`710022002`) | Wait for `retry_after_seconds` |
| `429 quota_exhausted` | Subscription quota exhausted | Wait for reset time, or redeem a reset card |
| `502 upstream_error` | Upstream connection failure | Check network; prefer `endpoint: samantha` |
| `signer_error` | External signer unreachable or invalid JSON | Check `signer_url`, run signer locally |
| Empty answer | Channel/format mismatch | Use `endpoint: samantha` |

`710022004` is not a quota error. The upstream stream carries
`extra.decision.type=verify` (for example
`verify_scene=doubao_message_web`), so the plugin reports
`verification_required`, scopes the cooldown to the credential, and fails fast
for other models until the window expires. Complete the verification with the
same session used by the auth file; verifying in a different app or browser
does not update the stored cookies.

## 8. Security Notes

- Auth files contain session cookies. Treat CPA's auth directory as a
  secret store and restrict its filesystem permissions.
- The plugin never logs sensitive request bodies or full cookie values.
- Prefer a signer bound to `127.0.0.1`; remote signers must use HTTPS.
