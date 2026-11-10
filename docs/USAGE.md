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
      # public_base_url: https://cpa.example.com
```

`public_base_url` is needed for remote panels or reverse proxies. Local panels
default to `http://127.0.0.1:<port>`; the plugin never appends resource paths to
CPA's OAuth callback URL.

## 2. Authentication

### Option A: Desktop Session Import (Recommended)

1. Open the panel's plugin / OAuth page and choose **豆姐（桌面端导入）**.
2. The plugin reads the local Windows desktop client session and writes an auth
   credential immediately.
3. No mobile scanning required, ensuring the imported account matches your
   active desktop profile.

Doubao keeps its cookie database open while running. If the session is locked,
the plugin opens a no-QR import page instead of failing:

1. Quit Doubao completely from the notification-area tray.
2. Reopen the import page and click **重新读取桌面端**.

The import link expires with the normal five-minute login flow.

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

Choose **豆姐（国内版）** or **豆姐国际版（Dola）**. The plugin opens the system
default Chromium browser in an isolated profile and the matching login page.
Enter the phone number, password or SMS code there. A loopback-only DevTools
connection reads the resulting cookies; the plugin never receives or stores the
password. The browser and temporary profile are closed after success or expiry.

This mode requires a desktop Windows session and a Chromium-compatible default
browser. Set `DOUJIE_BROWSER_PATH` if the browser is installed in a non-standard
location.

### Session Lifetime

Sessions are long-lived. When upstream rejects credentials (HTTP 401),
delete the auth file and sign in or import again.

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
  "headers": {"Cookie": ["..."]},
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
- `Host`, `Content-Length`, `Connection` and `Cookie` are managed by the plugin
  and ignored from signer responses.
- Keep the signer service bound to loopback (`127.0.0.1`).

## 5. Protocol Mapping

| Client | Request fields read | Response fields produced |
| --- | --- | --- |
| Anthropic Messages | `system`, `messages[].content`, `thinking` | `message_start` / `thinking_delta` / `text_delta` / `message_stop` |
| Chat Completions | `messages[].content`, `reasoning_effort` | `chat.completion.chunk` with `content` and `reasoning_content` |
| Responses | `instructions`, `input`, `reasoning.effort` | `response.output_text.delta`, `response.completed` |

Token counts are estimated where upstream streaming chunks do not supply usage totals.

## 5.1 Video Models

The plugin advertises `doubao-video`, `seedance-2.5`, `seedance-2.0`,
`seedance-2.0-fast`, and `seedance-2.0-mini`. Use a normal Chat Completions
request. The first request returns a short plan; send `"confirm": true` (or a
follow-up user message such as `确认生成`) to submit exactly one video task. The
final MP4 URL is returned in the assistant message.

Video generation uses the official Creative Video skill and web-session quota.
The plugin resolves the account's real model options from
`/samantha/skill/pack`, validates the requested model, and submits the
`[创意视频](skill://doubao-creative-video?type=2&id=346073050)` mention with
`mention_skill_list` / `mention_ext` / `resourcemention`. `Seedance 2.0`
(`seedance_v2.0_std`) requires an enhanced subscription; use Fast or Mini if the
account does not have it. `query_video_gen_info.remain_count` is not the video
quota; the server still checks subscription and the 7-day video quota. It is
currently text-to-video only.

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
| `429 quota_exhausted` | Quota or rate limit hit | Wait for reset time, or redeem a reset card |
| `502 upstream_error` | Upstream connection failure | Check network; prefer `endpoint: samantha` |
| QR image 404 | Flow expired (5 minutes) or plugin restarted | Start the sign-in again |
| `signer_error` | External signer unreachable or invalid JSON | Check `signer_url`, run signer locally |
| Empty answer | Channel/format mismatch | Use `endpoint: samantha` |

## 8. Security Notes

- Auth files contain session cookies. Treat CPA's auth directory as a
  secret store and restrict its filesystem permissions.
- The plugin never logs sensitive request bodies or full cookie values.
- Keep external signers bound to `127.0.0.1` and never expose them to public networks.
