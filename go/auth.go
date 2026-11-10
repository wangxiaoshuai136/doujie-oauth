package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const (
	doubaoRoot  = "https://www.doubao.com"
	passportAID = "497858"
	chromeUA    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"
	chromiumVer = "148.0.7816.0"
	flowsTTL    = 5 * time.Minute
)

type loginFlow struct {
	Site     string
	Token    string
	CSRF     string
	Cookies  map[string]string
	Expires  time.Time
	Browser  *browserLogin
	Imported *pluginapi.AuthData
}

var flowMu sync.Mutex
var flowStore = map[string]*loginFlow{}

func putFlow(id string, flow *loginFlow) {
	flowMu.Lock()
	defer flowMu.Unlock()
	now := time.Now()
	for key, item := range flowStore {
		if item == nil || item.Expires.Before(now) {
			delete(flowStore, key)
		}
	}
	flowStore[id] = flow
}

func takeFlow(id string) *loginFlow {
	flowMu.Lock()
	defer flowMu.Unlock()
	flow := flowStore[id]
	delete(flowStore, id)
	return flow
}

func peekFlow(id string) *loginFlow {
	flowMu.Lock()
	defer flowMu.Unlock()
	flow := flowStore[id]
	if flow == nil || flow.Expires.Before(time.Now()) {
		delete(flowStore, id)
		return nil
	}
	return flow
}

const (
	browserEntryID         = "doubao-account"
	dolaBrowserEntryID     = "dola-account"
	domesticQREntryID      = "doubao-qr"
	internationalQREntryID = "dola-qr"
)

func closeAllFlows() {
	flowMu.Lock()
	defer flowMu.Unlock()
	for id, flow := range flowStore {
		if flow != nil && flow.Browser != nil {
			flow.Browser.Close()
		}
		delete(flowStore, id)
	}
}

func browserHeaders(extra http.Header) http.Header {
	return browserHeadersForSite(siteCN, extra)
}

func browserHeadersForSite(site string, extra http.Header) http.Header {
	profile := profileForSite(site)
	language := "zh-CN,zh;q=0.9"
	if profile.Language != "zh" {
		language = profile.Language + ",en;q=0.8"
	}
	h := http.Header{
		"User-Agent":      {chromeUA},
		"Accept":          {"application/json, text/plain, */*"},
		"Referer":         {profile.Root + "/chat/login"},
		"Origin":          {profile.Root},
		"Accept-Language": {language},
	}
	for k, v := range extra {
		h[k] = append([]string(nil), v...)
	}
	return h
}

func mergeCookies(jar map[string]string, headers http.Header) {
	for _, raw := range headers.Values("Set-Cookie") {
		part := strings.TrimSpace(strings.SplitN(raw, ";", 2)[0])
		name, value, ok := strings.Cut(part, "=")
		if ok && strings.TrimSpace(name) != "" {
			jar[strings.TrimSpace(name)] = strings.TrimSpace(value)
		}
	}
}

func httpGet(sender httpSender, target string, headers http.Header, jar map[string]string) (pluginapi.HTTPResponse, error) {
	if headers == nil {
		headers = http.Header{}
	}
	if len(jar) > 0 && headers.Get("Cookie") == "" {
		headers.Set("Cookie", cookieHeader(jar))
	}
	resp, err := sender.Do(pluginapi.HTTPRequest{Method: http.MethodGet, URL: target, Headers: headers})
	if err != nil {
		return resp, err
	}
	mergeCookies(jar, resp.Headers)
	return resp, nil
}

func startLogin(req pluginapi.AuthLoginStartRequest, sender httpSender) (pluginapi.AuthLoginStartResponse, error) {
	var out pluginapi.AuthLoginStartResponse
	if req.Host.AuthDir == "" {
		return out, errors.New("CPA auth directory unavailable")
	}
	key, err := loadStateKey(req.Host.AuthDir)
	if err != nil {
		return out, err
	}
	switch req.Provider {
	case entryID, browserEntryID:
		return startBrowserAccountLogin(req, key, siteCN)
	case internationalEntryID, dolaBrowserEntryID:
		return startBrowserAccountLogin(req, key, siteGlobal)
	}
	if req.Provider == desktopEntryID {
		return startDesktopImport(req, sender, key)
	}
	site := siteCN
	if req.Provider == internationalQREntryID {
		site = siteGlobal
	}
	profile := profileForSite(site)
	jar := map[string]string{}
	resp, err := httpGet(sender, profile.Root+"/", browserHeadersForSite(site, nil), jar)
	if err != nil {
		return out, errors.New("cannot reach Doubao login")
	}
	if resp.StatusCode >= 400 {
		return out, fmt.Errorf("Doubao login page returned HTTP %d", resp.StatusCode)
	}
	csrfResp, err := httpGet(sender, profile.Root+"/passport/safe/csrf_token/?aid="+profile.PassportAID, browserHeadersForSite(site, nil), jar)
	if err != nil {
		return out, errors.New("cannot request Doubao CSRF token")
	}
	csrf := jar["passport_csrf_token"]
	if csrf == "" {
		csrf = jar["passport_csrf_token_default"]
	}
	var csrfEnvelope struct {
		Data struct {
			PassportCSRFToken string `json:"passport_csrf_token"`
		} `json:"data"`
	}
	if json.Unmarshal(csrfResp.Body, &csrfEnvelope) == nil && csrfEnvelope.Data.PassportCSRFToken != "" {
		csrf = csrfEnvelope.Data.PassportCSRFToken
	}
	if csrf == "" {
		return out, errors.New("Doubao did not return a CSRF token")
	}
	next := url.QueryEscape(profile.Root)
	qrURL := profile.Root + "/passport/web/get_qrcode/?next=" + next + "&aid=" + profile.PassportAID
	qrResp, err := httpGet(sender, qrURL, browserHeadersForSite(site, http.Header{"X-Tt-Passport-Csrf-Token": {csrf}}), jar)
	if err != nil {
		return out, errors.New("cannot request Doubao login QR code")
	}
	var qrEnvelope struct {
		Data struct {
			Token     string `json:"token"`
			QRCode    string `json:"qrcode"`
			QRCodeURL string `json:"qrcode_url"`
		} `json:"data"`
	}
	if json.Unmarshal(qrResp.Body, &qrEnvelope) != nil || qrEnvelope.Data.Token == "" {
		return out, errors.New("Doubao returned an invalid login QR code")
	}
	png, err := loadQRImage(sender, qrEnvelope.Data.QRCode, qrEnvelope.Data.QRCodeURL)
	if err != nil {
		return out, err
	}
	flowID, err := randomHex(16)
	if err != nil {
		return out, errors.New("cannot generate login state")
	}
	expires := time.Now().Add(flowsTTL)
	putFlow(flowID, &loginFlow{Site: site, Token: qrEnvelope.Data.Token, CSRF: csrf, Cookies: jar, Expires: expires})
	state, err := sealState(key, flowID, expires)
	if err != nil {
		return out, err
	}
	qrDataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	out = pluginapi.AuthLoginStartResponse{
		Provider:  credentialProvider,
		URL:       qrDataURL,
		State:     state,
		ExpiresAt: expires.UTC(),
		Metadata: map[string]any{
			"site":        site,
			"qr_base64":   base64.StdEncoding.EncodeToString(png),
			"qr_mime":     "image/png",
			"poll_hint":   "scan with the mobile app",
			"expires_sec": int(flowsTTL / time.Second),
		},
	}
	return out, nil
}

func absoluteBase(publicBase, hostBase string) string {
	// public_base_url is an operator override and may deliberately include a
	// reverse-proxy path. CPA's login BaseURL, however, is the OAuth callback
	// URL, not an asset root; only its origin belongs in public resource links.
	if u, ok := httpBaseURL(publicBase); ok {
		return strings.TrimSuffix(u.String(), "/")
	}
	if u, ok := httpBaseURL(hostBase); ok {
		u.Path = ""
		u.RawPath = ""
		u.RawQuery = ""
		u.ForceQuery = false
		u.Fragment = ""
		return strings.TrimSuffix(u.String(), "/")
	}
	return ""
}

// safeLoginRedirect only follows HTTPS redirects on the Doubao-owned domain for
// the active site. Flow cookies are never sent to an upstream-supplied origin.
func safeLoginRedirect(raw, site string) (string, bool) {
	u, ok := httpBaseURL(raw)
	if !ok || u.Scheme != "https" || u.User != nil {
		return "", false
	}
	if port := u.Port(); port != "" && port != "443" {
		return "", false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	suffix := "doubao.com"
	if normalizeSite(site) == siteGlobal {
		suffix = "dola.com"
	}
	if host != suffix && !strings.HasSuffix(host, "."+suffix) {
		return "", false
	}
	return u.String(), true
}

func decodeQRCode(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("empty QR code")
	}
	if strings.HasPrefix(raw, "data:") {
		_, payload, ok := strings.Cut(raw, ",")
		if !ok {
			return nil, errors.New("invalid QR data URI")
		}
		raw = payload
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return nil, errors.New("remote QR image")
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(raw); err == nil && len(b) > 8 {
			return b, nil
		}
	}
	return nil, errors.New("invalid QR base64")
}

func pollLogin(req pluginapi.AuthLoginPollRequest, sender httpSender) (pluginapi.AuthLoginPollResponse, error) {
	pending := pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusPending}
	if req.Host.AuthDir == "" {
		return pending, errors.New("CPA auth directory unavailable")
	}
	key, err := loadStateKey(req.Host.AuthDir)
	if err != nil {
		return pending, err
	}
	flowID, expires, err := openState(key, req.State)
	if err != nil {
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: "登录状态无效，请重新开始"}, nil
	}
	if time.Now().After(expires) {
		if flow := peekFlow(flowID); flow != nil && flow.Browser != nil {
			flow.Browser.Close()
		}
		takeFlow(flowID)
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: "登录流程已过期，请重新开始"}, nil
	}
	flow := peekFlow(flowID)
	if flow == nil {
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: "登录流程已失效，请重新开始"}, nil
	}
	if flow.Imported != nil {
		takeFlow(flowID)
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusSuccess, Auth: *flow.Imported}, nil
	}
	if flow.Browser != nil {
		cookies, browserErr := flow.Browser.PollCookies()
		if browserErr == nil && strings.TrimSpace(cookies["sessionid"]) != "" {
			auth, authErr := authFromCookies(cookies, flow.Site)
			if authErr != nil {
				return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: authErr.Error()}, nil
			}
			auth.Metadata["oauth_provider"] = entryID
			if normalizeSite(flow.Site) == siteGlobal {
				auth.Metadata["oauth_provider"] = internationalEntryID
			}
			flow.Browser.Close()
			takeFlow(flowID)
			return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusSuccess, Auth: auth}, nil
		}
		pending.Message = "请在浏览器中完成账号登录"
		return pending, nil
	}
	profile := profileForSite(flow.Site)
	next := url.QueryEscape(profile.Root)
	target := profile.Root + "/passport/web/check_qrconnect/?next=" + next + "&token=" + url.QueryEscape(flow.Token) + "&aid=" + profile.PassportAID
	resp, err := httpGet(sender, target, browserHeadersForSite(flow.Site, http.Header{"X-Tt-Passport-Csrf-Token": {flow.CSRF}}), flow.Cookies)
	if err != nil {
		return pending, nil
	}
	var envelope struct {
		Message   string `json:"message"`
		ErrorCode int    `json:"error_code"`
		Data      struct {
			Status      string `json:"status"`
			ErrorCode   int    `json:"error_code"`
			Description string `json:"description"`
			RedirectURL string `json:"redirect_url"`
		} `json:"data"`
	}
	if json.Unmarshal(resp.Body, &envelope) != nil {
		return pending, nil
	}
	code := envelope.Data.ErrorCode
	if code == 0 && envelope.ErrorCode != 0 {
		code = envelope.ErrorCode
	}
	if code != 0 {
		if strings.Contains(strings.ToLower(envelope.Data.Description), "expired") || strings.Contains(envelope.Data.Description, "过期") {
			takeFlow(flowID)
			return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: "二维码已过期，请重新开始"}, nil
		}
		return pending, nil
	}
	switch envelope.Data.Status {
	case "new":
		pending.Message = "等待扫码"
		return pending, nil
	case "scanned":
		pending.Message = "已扫码，请在手机上确认"
		return pending, nil
	case "expired":
		takeFlow(flowID)
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: "二维码已过期，请重新开始"}, nil
	case "confirmed":
		if envelope.Data.RedirectURL != "" {
			if target, ok := safeLoginRedirect(envelope.Data.RedirectURL, flow.Site); ok {
				_, _ = httpGet(sender, target, browserHeaders(nil), flow.Cookies)
			}
		}
		if strings.TrimSpace(flow.Cookies["sessionid"]) == "" {
			return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: "登录已确认，但未取到会话凭据"}, nil
		}
		auth, err := authFromCookies(flow.Cookies, flow.Site)
		if err != nil {
			return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: err.Error()}, nil
		}
		takeFlow(flowID)
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusSuccess, Auth: auth}, nil
	default:
		pending.Message = "等待扫码"
		return pending, nil
	}
}

func authFromCookies(jar map[string]string, site string) (pluginapi.AuthData, error) {
	deviceID, webID, fp := newDeviceIdentity()
	cookies := map[string]string{}
	for k, v := range jar {
		if strings.TrimSpace(v) != "" {
			cookies[k] = v
		}
	}
	site = normalizeSite(site)
	c := credential{Type: credentialProvider, Site: site, Cookies: cookies, DeviceID: deviceID, WebID: webID, FP: fp, LoginAt: time.Now().Unix()}
	raw, err := json.Marshal(c)
	if err != nil {
		return pluginapi.AuthData{}, errors.New("cannot encode credential")
	}
	sum := sha256.Sum256([]byte(cookies["sessionid"]))
	prefix := "doubao-"
	providerEntry := entryID
	labelPrefix := "豆姐账号 "
	if site == siteGlobal {
		prefix = "dola-"
		providerEntry = internationalEntryID
		labelPrefix = "豆姐国际版账号 "
	}
	id := prefix + hex.EncodeToString(sum[:12]) + ".json"
	label := labelPrefix + strings.ToUpper(hex.EncodeToString(sum[:3]))
	return pluginapi.AuthData{
		Provider:    credentialProvider,
		ID:          id,
		FileName:    id,
		Label:       label,
		StorageJSON: raw,
		Metadata:    map[string]any{"email": label, "type": credentialProvider, "oauth_provider": providerEntry, "site": site},
	}, nil
}

func parseAuth(req pluginapi.AuthParseRequest) pluginapi.AuthParseResponse {
	if req.Provider != entryID && req.Provider != internationalEntryID && req.Provider != desktopEntryID &&
		req.Provider != browserEntryID && req.Provider != dolaBrowserEntryID &&
		req.Provider != domesticQREntryID && req.Provider != internationalQREntryID && req.Provider != credentialProvider {
		return pluginapi.AuthParseResponse{}
	}
	var persisted struct {
		Type  string `json:"type"`
		Email string `json:"email"`
	}
	if json.Unmarshal(req.RawJSON, &persisted) != nil || persisted.Type != credentialProvider {
		return pluginapi.AuthParseResponse{}
	}
	c, err := readCredential(req.RawJSON)
	if err != nil {
		return pluginapi.AuthParseResponse{}
	}
	name := req.FileName
	if name == "" {
		name = req.Path
	}
	metadata := persistedMetadata(req.RawJSON, c)
	metadata["email"] = persisted.Email
	metadata["type"] = credentialProvider
	metadata["site"] = c.site()
	metadata["oauth_provider"] = entryID
	if c.site() == siteGlobal {
		metadata["oauth_provider"] = internationalEntryID
	}
	return pluginapi.AuthParseResponse{Handled: true, Auth: pluginapi.AuthData{
		Provider:    credentialProvider,
		ID:          name,
		FileName:    req.FileName,
		Label:       persisted.Email,
		StorageJSON: append([]byte(nil), req.RawJSON...),
		Metadata:    metadata,
	}}
}

func persistedMetadata(raw []byte, own any) map[string]any {
	var metadata map[string]any
	if json.Unmarshal(raw, &metadata) != nil || metadata == nil {
		return make(map[string]any)
	}
	for _, key := range credentialJSONKeys(own) {
		delete(metadata, key)
	}
	return metadata
}

func credentialJSONKeys(own any) []string {
	t := reflect.TypeOf(own)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	keys := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		keys = append(keys, name)
	}
	return keys
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func loginError(err error) error {
	return pluginabi.NewError("auth_error", err.Error(), 502)
}

func loadQRImage(sender httpSender, candidates ...string) ([]byte, error) {
	for _, raw := range candidates {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
			resp, err := sender.Do(pluginapi.HTTPRequest{Method: http.MethodGet, URL: raw, Headers: browserHeaders(nil)})
			if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 && len(resp.Body) > 8 {
				return resp.Body, nil
			}
			continue
		}
		if png, err := decodeQRCode(raw); err == nil {
			return png, nil
		}
	}
	return nil, errors.New("Doubao login QR code has no image")
}

func startDesktopImport(req pluginapi.AuthLoginStartRequest, sender httpSender, key []byte) (pluginapi.AuthLoginStartResponse, error) {
	var out pluginapi.AuthLoginStartResponse
	if req.Host.AuthDir == "" {
		return out, errors.New("CPA auth directory unavailable")
	}
	cookies, err := readDesktopCookies()
	var authData pluginapi.AuthData
	if err == nil {
		authData, err = importDesktopAuth(sender, cookies)
		if err != nil {
			return out, err
		}
	} else {
		return out, errors.New("桌面端 Cookies 正在被占用，请从任务栏托盘完全退出豆包后重试")
	}
	flowID, err := randomHex(16)
	if err != nil {
		return out, errors.New("cannot generate login state")
	}
	expires := time.Now().Add(flowsTTL)
	flow := &loginFlow{Site: siteCN, Expires: expires, Imported: &authData}
	putFlow(flowID, flow)
	state, err := sealState(key, flowID, expires)
	if err != nil {
		return out, err
	}
	cfg := getConfig()
	base := absoluteBase(cfg.PublicBaseURL, req.BaseURL)
	resourceURL := base + resourcePrefix + "/imported.html"
	metadata := map[string]any{"source": "desktop-import"}
	metadata["label"] = authData.Label
	out = pluginapi.AuthLoginStartResponse{
		Provider:  credentialProvider,
		URL:       resourceURL,
		State:     state,
		ExpiresAt: expires.UTC(),
		Metadata:  metadata,
	}
	return out, nil
}

func startBrowserAccountLogin(req pluginapi.AuthLoginStartRequest, key []byte, site string) (pluginapi.AuthLoginStartResponse, error) {
	var out pluginapi.AuthLoginStartResponse
	browser, err := startBrowserLogin(site)
	if err != nil {
		return out, fmt.Errorf("cannot open browser sign-in: %w", err)
	}
	flowID, err := randomHex(16)
	if err != nil {
		browser.Close()
		return out, errors.New("cannot generate login state")
	}
	expires := time.Now().Add(flowsTTL)
	putFlow(flowID, &loginFlow{Site: normalizeSite(site), Expires: expires, Browser: browser})
	state, err := sealState(key, flowID, expires)
	if err != nil {
		browser.Close()
		takeFlow(flowID)
		return out, err
	}
	base := absoluteBase(getConfig().PublicBaseURL, req.BaseURL)
	return pluginapi.AuthLoginStartResponse{
		Provider:  credentialProvider,
		URL:       base + resourcePrefix + "/account.html",
		State:     state,
		ExpiresAt: expires.UTC(),
		Metadata: map[string]any{
			"source":  "browser-login",
			"message": "已打开浏览器，请使用手机号、密码或短信验证码完成登录",
		},
	}, nil
}
