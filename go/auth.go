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
	Token   string
	CSRF    string
	Cookies map[string]string
	Expires time.Time
	QRPNG   []byte
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

func newestFlow() *loginFlow {
	flowMu.Lock()
	defer flowMu.Unlock()
	var newest *loginFlow
	var newestAt time.Time
	for _, flow := range flowStore {
		if flow == nil || flow.Expires.Before(time.Now()) || len(flow.QRPNG) == 0 {
			continue
		}
		if newest == nil || flow.Expires.After(newestAt) {
			newest = flow
			newestAt = flow.Expires
		}
	}
	return newest
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

func browserHeaders(extra http.Header) http.Header {
	h := http.Header{
		"User-Agent":      {chromeUA},
		"Accept":          {"application/json, text/plain, */*"},
		"Referer":         {doubaoRoot + "/chat/login"},
		"Origin":          {doubaoRoot},
		"Accept-Language": {"zh-CN,zh;q=0.9"},
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
	resp, err := sender.Do(pluginapi.HTTPRequest{Method: http.MethodGet, URL: target, Headers: headers})
	if err != nil {
		return resp, err
	}
	mergeCookies(jar, resp.Headers)
	return resp, nil
}

func startLogin(req pluginapi.AuthLoginStartRequest, sender httpSender) (pluginapi.AuthLoginStartResponse, error) {
	var out pluginapi.AuthLoginStartResponse
	cfg := getConfig()
	if req.Host.AuthDir == "" {
		return out, errors.New("CPA auth directory unavailable")
	}
	key, err := loadStateKey(req.Host.AuthDir)
	if err != nil {
		return out, err
	}
	jar := map[string]string{}
	resp, err := httpGet(sender, doubaoRoot+"/", browserHeaders(nil), jar)
	if err != nil {
		return out, errors.New("cannot reach Doubao login")
	}
	if resp.StatusCode >= 400 {
		return out, fmt.Errorf("Doubao login page returned HTTP %d", resp.StatusCode)
	}
	csrfResp, err := httpGet(sender, doubaoRoot+"/passport/safe/csrf_token/?aid="+passportAID, browserHeaders(nil), jar)
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
	next := url.QueryEscape(doubaoRoot)
	qrURL := doubaoRoot + "/passport/web/get_qrcode/?next=" + next + "&aid=" + passportAID
	qrResp, err := httpGet(sender, qrURL, browserHeaders(http.Header{"X-Tt-Passport-Csrf-Token": {csrf}}), jar)
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
	putFlow(flowID, &loginFlow{Token: qrEnvelope.Data.Token, CSRF: csrf, Cookies: jar, Expires: expires, QRPNG: png})
	state, err := sealState(key, flowID, expires)
	if err != nil {
		return out, err
	}
	base := absoluteBase(cfg.PublicBaseURL, req.BaseURL)
	qrPath := resourcePrefix + "/qr.png"
	out = pluginapi.AuthLoginStartResponse{
		Provider:  credentialProvider,
		URL:       base + qrPath,
		State:     state,
		ExpiresAt: expires.UTC(),
		Metadata: map[string]any{
			"qr_path":     qrPath,
			"qr_base64":   base64.StdEncoding.EncodeToString(png),
			"qr_mime":     "image/png",
			"poll_hint":   "scan with the Doubao mobile app",
			"expires_sec": int(flowsTTL / time.Second),
		},
	}
	return out, nil
}

func absoluteBase(candidates ...string) string {
	for _, raw := range candidates {
		if u, ok := httpBaseURL(raw); ok {
			return strings.TrimSuffix(u.String(), "/")
		}
	}
	return ""
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
		takeFlow(flowID)
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: "二维码已过期，请重新开始"}, nil
	}
	flow := peekFlow(flowID)
	if flow == nil {
		return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: "登录流程已失效，请重新开始"}, nil
	}
	next := url.QueryEscape(doubaoRoot)
	target := doubaoRoot + "/passport/web/check_qrconnect/?next=" + next + "&token=" + url.QueryEscape(flow.Token) + "&aid=" + passportAID
	resp, err := httpGet(sender, target, browserHeaders(http.Header{"X-Tt-Passport-Csrf-Token": {flow.CSRF}}), flow.Cookies)
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
			_, _ = httpGet(sender, envelope.Data.RedirectURL, browserHeaders(nil), flow.Cookies)
		}
		if strings.TrimSpace(flow.Cookies["sessionid"]) == "" {
			return pluginapi.AuthLoginPollResponse{Status: pluginapi.AuthLoginStatusError, Message: "登录已确认，但未取到会话凭据"}, nil
		}
		auth, err := authFromCookies(flow.Cookies)
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

func authFromCookies(jar map[string]string) (pluginapi.AuthData, error) {
	deviceID, webID, fp := newDeviceIdentity()
	cookies := map[string]string{}
	for k, v := range jar {
		if strings.TrimSpace(v) != "" {
			cookies[k] = v
		}
	}
	c := credential{Type: credentialProvider, Cookies: cookies, DeviceID: deviceID, WebID: webID, FP: fp, LoginAt: time.Now().Unix()}
	raw, err := json.Marshal(c)
	if err != nil {
		return pluginapi.AuthData{}, errors.New("cannot encode credential")
	}
	sum := sha256.Sum256([]byte(cookies["sessionid"]))
	id := "doubao-" + hex.EncodeToString(sum[:12]) + ".json"
	label := "豆包账号 " + strings.ToUpper(hex.EncodeToString(sum[:3]))
	return pluginapi.AuthData{
		Provider:    credentialProvider,
		ID:          id,
		FileName:    id,
		Label:       label,
		StorageJSON: raw,
		Metadata:    map[string]any{"email": label, "type": credentialProvider, "oauth_provider": entryID},
	}, nil
}

func parseAuth(req pluginapi.AuthParseRequest) pluginapi.AuthParseResponse {
	if req.Provider != entryID && req.Provider != credentialProvider {
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
	metadata["oauth_provider"] = entryID
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

