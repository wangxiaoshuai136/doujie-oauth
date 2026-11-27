package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	browserLoginTempPrefix     = "doujie-browser-"
	browserLoginHTTPTimeout    = 5 * time.Second
	browserLoginCommandTimeout = 8 * time.Second
	browserLoginCloseTimeout   = 2 * time.Second
	browserLoginCleanupTimeout = 4 * time.Second
)

type browserLogin struct {
	Port        int
	UserDataDir string
	PID         int
	Site        string
	WSURL       string
	client      *cdpClient
	mu          sync.Mutex
	process     *os.Process
	closed      bool
}

type cdpClient struct {
	conn           *websocket.Conn
	nextID         int64
	networkEnabled bool
}

type cdpCookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
}

type devToolsTarget struct {
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type cdpRequest struct {
	ID     int64  `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type cdpResponse struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *cdpError       `json:"error,omitempty"`
}

type cdpError struct {
	Code int `json:"code"`
}

// startBrowserLogin launches the user's default Chromium browser with an
// isolated profile and a loopback-only DevTools endpoint. When running as a
// service in session 0 it uses the active console user's profile and token so
// both the default-browser choice and the window belong to that user.
func startBrowserLogin(site string) (*browserLogin, error) {
	return startBrowserLoginWithBrowser(site, "auto")
}

func (b *browserLogin) PollCookies() (map[string]string, error) {
	if b == nil {
		return nil, errors.New("browser login is not initialized")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, errors.New("browser login is closed")
	}
	if b.Port <= 0 {
		return nil, errors.New("browser debugging port is unavailable")
	}

	if b.client == nil {
		b.WSURL = ""
		target, err := b.findPageTarget()
		if err != nil {
			return nil, err
		}
		client, err := dialCDP(target.WebSocketDebuggerURL)
		if err != nil {
			return nil, err
		}
		b.WSURL = target.WebSocketDebuggerURL
		b.client = client
	}

	cookies, err := b.client.allCookies()
	if err != nil {
		b.client.close()
		b.client = nil
		b.WSURL = ""
		return nil, err
	}
	result := cookieMapFromCDP(cookies, b.Site)
	if result["sessionid"] == "" {
		return map[string]string{}, nil
	}
	return result, nil
}

// Close asks the browser to exit, tears down the DevTools connection, kills the
// browser process if it is still running, and removes the temporary profile.
func (b *browserLogin) Close() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true

	if b.client != nil {
		_, _ = b.client.call("Browser.close", nil, browserLoginCloseTimeout)
		b.client.close()
		b.client = nil
	}
	b.WSURL = ""

	if b.process != nil {
		if !waitProcessExit(b.PID, 1500*time.Millisecond) {
			killBrowserProcess(b.process, b.PID)
			_ = waitProcessExit(b.PID, time.Second)
		}
		_ = b.process.Release()
		b.process = nil
	} else if b.PID > 0 {
		if process, err := os.FindProcess(b.PID); err == nil {
			if !waitProcessExit(b.PID, 1500*time.Millisecond) {
				killBrowserProcess(process, b.PID)
				_ = waitProcessExit(b.PID, time.Second)
			}
			_ = process.Release()
		}
	}
	b.PID = 0

	if b.UserDataDir != "" {
		removeUserDataDir(b.UserDataDir)
	}
	b.UserDataDir = ""
}

func (b *browserLogin) findPageTarget() (devToolsTarget, error) {
	client := &http.Client{Timeout: browserLoginHTTPTimeout}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/list", b.Port))
	if err != nil {
		return devToolsTarget{}, fmt.Errorf("query browser targets: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return devToolsTarget{}, fmt.Errorf("query browser targets: unexpected HTTP status %d", response.StatusCode)
	}
	var targets []devToolsTarget
	if err := json.NewDecoder(response.Body).Decode(&targets); err != nil {
		return devToolsTarget{}, errors.New("decode browser targets: invalid response")
	}
	target, ok := pickPageTarget(targets, b.Port, b.Site)
	if !ok {
		return devToolsTarget{}, errors.New("browser login page is not ready")
	}
	return target, nil
}

func (c *cdpClient) allCookies() ([]cdpCookie, error) {
	if c == nil {
		return nil, errors.New("browser debugging socket is unavailable")
	}
	if !c.networkEnabled {
		if _, err := c.call("Network.enable", nil, browserLoginCommandTimeout); err != nil {
			return nil, err
		}
		c.networkEnabled = true
	}
	raw, err := c.call("Network.getAllCookies", nil, browserLoginCommandTimeout)
	if err != nil {
		return nil, err
	}
	var result struct {
		Cookies []cdpCookie `json:"cookies"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, errors.New("decode browser cookies: invalid response")
	}
	return result.Cookies, nil
}

func (c *cdpClient) call(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	if c == nil || c.conn == nil {
		return nil, errors.New("browser debugging socket is unavailable")
	}
	if timeout <= 0 {
		timeout = browserLoginCommandTimeout
	}
	c.nextID++
	id := c.nextID
	if err := c.conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return nil, errors.New("set browser command deadline")
	}
	if err := c.conn.WriteJSON(cdpRequest{ID: id, Method: method, Params: params}); err != nil {
		return nil, fmt.Errorf("send browser command %s: %w", method, err)
	}
	deadline := time.Now().Add(timeout)
	for {
		if err := c.conn.SetReadDeadline(deadline); err != nil {
			return nil, errors.New("set browser response deadline")
		}
		var response cdpResponse
		if err := c.conn.ReadJSON(&response); err != nil {
			return nil, fmt.Errorf("read browser response %s: %w", method, err)
		}
		if response.ID != id {
			continue
		}
		if response.Error != nil {
			return nil, fmt.Errorf("browser command %s failed (code %d)", method, response.Error.Code)
		}
		return response.Result, nil
	}
}

func (c *cdpClient) close() {
	if c == nil || c.conn == nil {
		return
	}
	_ = c.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second),
	)
	_ = c.conn.Close()
	c.conn = nil
}

func dialCDP(wsURL string) (*cdpClient, error) {
	dialer := websocket.Dialer{HandshakeTimeout: browserLoginHTTPTimeout}
	conn, response, err := dialer.Dial(wsURL, nil)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("connect to browser debugging socket: %w", err)
	}
	return &cdpClient{conn: conn}, nil
}

func pickPageTarget(targets []devToolsTarget, port int, site string) (devToolsTarget, bool) {
	var siteChat, siteAny, fallback *devToolsTarget
	for i := range targets {
		target := &targets[i]
		if target.Type != "page" || target.WebSocketDebuggerURL == "" {
			continue
		}
		if !devToolsWSURLForPort(target.WebSocketDebuggerURL, port) {
			continue
		}
		if fallback == nil {
			fallback = target
		}
		host, path := targetURLParts(target.URL)
		if hostMatchesSite(host, site) {
			if siteAny == nil {
				siteAny = target
			}
			if strings.HasPrefix(path, "/chat") && siteChat == nil {
				siteChat = target
			}
		}
	}
	for _, target := range []*devToolsTarget{siteChat, siteAny, fallback} {
		if target != nil {
			return *target, true
		}
	}
	return devToolsTarget{}, false
}

func hostMatchesSite(host, site string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	root, err := url.Parse(profileForSite(site).Root)
	if err != nil {
		return false
	}
	base := strings.TrimPrefix(strings.ToLower(root.Hostname()), "www.")
	return host == base || strings.HasSuffix(host, "."+base)
}

func targetURLParts(raw string) (string, string) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}
	return strings.ToLower(parsed.Hostname()), parsed.Path
}

func devToolsWSURLForPort(raw string, port int) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "ws" || parsed.Port() != strconv.Itoa(port) {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

// cookieMapFromCDP keeps only cookies for the selected portal and returns an
// empty map until the session cookie appears.
func cookieMapFromCDP(cookies []cdpCookie, site string) map[string]string {
	result := map[string]string{}
	best := map[string]int{}
	for _, cookie := range cookies {
		name := strings.TrimSpace(cookie.Name)
		if name == "" || strings.TrimSpace(cookie.Value) == "" {
			continue
		}
		if !cookieDomainAllowed(cookie.Domain, site) {
			continue
		}
		score := cookiePreference(cookie)
		if previous, ok := best[name]; ok && score <= previous {
			continue
		}
		result[name] = cookie.Value
		best[name] = score
	}
	if result["sessionid"] == "" {
		return map[string]string{}
	}
	return result
}

func cookieDomainAllowed(domain, site string) bool {
	domain = normalizeCookieDomain(domain)
	return hostMatchesSite(domain, site) ||
		domain == "bytedance.com" ||
		strings.HasSuffix(domain, ".bytedance.com")
}

func normalizeCookieDomain(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.TrimPrefix(domain, ".")
	domain = strings.TrimSuffix(domain, ".")
	return domain
}

func cookiePreference(cookie cdpCookie) int {
	domain := normalizeCookieDomain(cookie.Domain)
	score := 0
	switch {
	case domain == "doubao.com" || strings.HasSuffix(domain, ".doubao.com"):
		score = 2_000_000
	case domain == "dola.com" || strings.HasSuffix(domain, ".dola.com"):
		score = 2_000_000
	case domain == "bytedance.com" || strings.HasSuffix(domain, ".bytedance.com"):
		score = 1_000_000
	}
	score += strings.Count(domain, ".") * 10_000
	if cookie.Path == "/" {
		score += 1_000
	} else {
		score += len(cookie.Path)
	}
	return score
}

func freeLoopbackPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("reserve browser debugging port: %w", err)
	}
	defer listener.Close()
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("reserve browser debugging port: unexpected address")
	}
	return address.Port, nil
}

func removeUserDataDir(path string) {
	if path == "" || !filepath.IsAbs(path) {
		return
	}
	if !strings.HasPrefix(filepath.Base(path), browserLoginTempPrefix) {
		return
	}
	deadline := time.Now().Add(browserLoginCleanupTimeout)
	for {
		if err := os.RemoveAll(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}
