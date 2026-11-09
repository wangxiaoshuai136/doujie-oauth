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

### Option B: Mobile QR Sign-In

The plugin does not auto-detect regions. Choose either **豆姐（国内版）** or **豆姐国际版（Dola）** explicitly in the panel. The international entry uses `dola.com` and is subject to the upstream regional policy; it is optional and does not change the default domestic account.

1. Open the panel's plugin / OAuth page and select the desired entry.
2. The panel renders a QR image served by the plugin at
   `/v0/resource/plugins/doujie-oauth/qr.png` (or base64 metadata).
3. Scan it with the mobile app and confirm authorization.
4. The plugin completes the session handshake and writes a `doubao` auth file.

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

## 6. Quota Card and Reset Cards

The plugin synchronizes subscription quotas and reset cards using authorized
user sessions:

- Displays active `7d` and `5h` rolling limit windows with current usage numbers
  and reset timestamps.
- Displays available quota reset-card inventory.
- Provides a **Reset Quota** action to redeem a card and refresh quota immediately.
- Falls back to local request-count estimation for free-tier accounts lacking
  server-side subscription windows.

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
