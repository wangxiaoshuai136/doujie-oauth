# Operator guide

## 1. Install the library

Build or download `omc-doubao-oauth.{dll,dylib,so}` and put it in the directory
CPA scans for plugins (`plugins.dir`). Enable the plugin in `config.yaml`:

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

`public_base_url` is only needed when your panel does not pass an absolute base
URL to the plugin. It is used to build the QR image address.

## 2. Sign in

1. Open the panel's plugin / OAuth page and start the `豆包` sign-in entry.
2. The panel opens a QR image served by the plugin at
   `/v0/resource/plugins/omc-doubao-oauth/qr.png` (the latest active flow).
3. Scan it with the Doubao mobile app and confirm on the phone.
4. The plugin polls `check_qrconnect`, follows the final redirect, and writes an
   auth file whose type is `doubao`.

If the panel cannot render the image URL, the login response metadata also
contains `qr_base64` (PNG) and `qr_path`; a panel that understands metadata can
render it directly.

### Session lifetime

Doubao sessions are long-lived but not refreshable by the plugin. When the
upstream returns 401/403, delete the auth file and sign in again. Do not reuse
one account's cookies across machines.

## 3. Upstream endpoints

### `samantha` (default)

```text
POST https://www.doubao.com/samantha/chat/completion?aid=582478&...
Content-Type: application/json
Cookie: sessionid=...; ttwid=...; ...
```

The body is the plain `messages` / `completion_option` object. This endpoint
works with cookies and, when present, the `msToken` cookie. `deep_think` maps to
`use_deep_think` and `use_auto_cot`.

### `chat`

```text
POST https://www.doubao.com/chat/completion?aid=582478&...
```

This is the desktop/CLI main entry. The body uses `client_meta` plus
`content_block` messages, and carries the CSRF cookie in
`x-tt-passport-csrf-token`. It normally requires ByteDance request signing
(`a_bogus`), so pair it with `signer: external`.

## 4. Signer service contract

The plugin sends one JSON object per request:

```json
{
  "method": "POST",
  "url": "https://www.doubao.com/chat/completion?aid=582478&...",
  "headers": {"Cookie": ["sessionid=...; ttwid=..."]},
  "body": "{\"client_meta\":{...}}"
}
```

It expects:

```json
{"headers": {"X-Bogus": "...", "msToken": "..."}}
```

Rules:

- The service must answer within the host HTTP timeout. A slow signer blocks the
  request.
- `Host`, `Content-Length`, `Connection` and `Cookie` are ignored by the plugin.
- Return only the headers you want merged. Unknown headers are passed through
  to Doubao, so keep the response minimal.

### Browser-backed signer sketch

The most reliable approach is a real page on `www.doubao.com` whose own fetch
hook computes `a_bogus`. A Node/Playwright helper can expose exactly the
contract above:

```js
// shape only: run a persistent browser context with a Doubao session
import http from "node:http";
import { chromium } from "playwright";

const page = await (await chromium.launchPersistentContext("./profile")).newPage();
await page.goto("https://www.doubao.com/chat/");

http.createServer(async (req, res) => {
  const input = JSON.parse(await new Promise(r => {
    let b = ""; req.on("data", d => b += d); req.on("end", () => r(b));
  }));
  // Let the page's own signing hook run and capture the headers it adds.
  const headers = await page.evaluate(async ({ url, body }) => {
    const response = await fetch(url, { method: "POST", body, headers: { "Content-Type": "application/json" } });
    return Object.fromEntries(response.headers); // replace with the captured request headers
  }, input);
  res.writeHead(200, { "Content-Type": "application/json" });
  res.end(JSON.stringify({ headers }));
}).listen(8787);
```

The sketch shows the shape, not a working signer: the hook must capture the
outgoing request headers (for example by patching `window.fetch`) rather than
the response headers. Keep the signer bound to `127.0.0.1` and never expose it
to the network.

## 5. Protocol mapping

| Client | Request fields read | Response fields produced |
| --- | --- | --- |
| Anthropic Messages | `system`, `messages[].content`, `thinking` | `message_start` / `thinking_delta` / `text_delta` / `message_stop` |
| Chat Completions | `messages[].content`, `reasoning_effort` | `chat.completion.chunk` with `content` and `reasoning_content` |
| Responses | `instructions`, `input`, `reasoning.effort` | `response.output_text.delta`, `response.completed` |

Token counts are estimates because Doubao's web endpoints do not return usage.

## 6. Quota card and reset cards

The quota card uses Doubao's account endpoints:

- `POST /alice/commerce/sale/subscription/quota/summary/` with
  `{"product_line":"membership"}` returns
  `data.window_limit_section.window_limit_groups[].window_limits[]`. Each window
  limit carries `start_time`, `end_time`, `used_amount`, `total_amount` and a
  unit, which maps to the panel's `5h` / `7d` buckets and reset times.
- `POST /alice/commerce/marketing/card/balance/` returns the reset-card stock
  (`available_count_by_card_key.pc_quota_reset_card` / `quota_reset_card`).
- `POST /alice/commerce/marketing/card/use/` consumes one card with
  `{"card_key":"pc_quota_reset_card","use_request_id":"<uuid>"}`.

These endpoints work with the session cookies only; the desktop/web client
signs chat traffic with `a_bogus`, but the commerce endpoints do not require it.
If the account has no subscription windows, the card is labeled as a local
request-based estimate.

## 7. Troubleshooting

| Symptom | Meaning | Action |
| --- | --- | --- |
| `401 reauth_required` | session expired or rejected | delete the auth file, sign in again |
| `429 quota_exhausted` | account quota or rate limit | wait, or use another account |
| `502 upstream_error` | Doubao rejected or broke the stream | check `endpoint`, retry, inspect CPA logs |
| QR image 404 | flow expired (5 minutes) or plugin restarted | start the sign-in again |
| `signer_error` | external signer unreachable or invalid JSON | check `signer_url`, run the signer locally |
| empty answer | endpoint/format mismatch | try `endpoint: samantha` first |

## 7. Security notes

- Auth files contain live `sessionid` cookies. Treat CPA's auth directory as a
  secret store and restrict its permissions.
- The plugin never logs request bodies or cookies.
- The external signer receives URLs and bodies. Run it on the same machine and
  keep it bound to loopback.



