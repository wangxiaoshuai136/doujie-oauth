//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/sys/windows"
)

const (
	browserLoginURL            = "https://www.doubao.com/chat/"
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

// startBrowserLogin launches a visible Edge/Chrome instance with an isolated
// profile and a loopback-only DevTools endpoint. When running as a service in
// session 0 it duplicates the active console user's token as a primary token
// so the browser appears on that user's desktop.
func startBrowserLogin() (*browserLogin, error) {
	port, err := freeLoopbackPort()
	if err != nil {
		return nil, err
	}

	var launchToken windows.Token
	var profile string
	if processRunsInSessionZero() {
		consoleToken, consoleProfile, err := activeConsoleUser()
		if err != nil {
			return nil, fmt.Errorf("locate the active desktop user: %w", err)
		}
		profile = consoleProfile
		launchToken, err = duplicatePrimaryToken(consoleToken)
		consoleToken.Close()
		if err != nil {
			return nil, err
		}
		defer launchToken.Close()
	} else {
		profile, err = currentUserProfile()
		if err != nil {
			return nil, err
		}
	}

	userDataDir, err := newBrowserUserDataDir(profile)
	if err != nil {
		return nil, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(userDataDir)
		}
	}()

	browserPath, err := resolveBrowserExecutable(profile)
	if err != nil {
		return nil, err
	}

	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-allow-origins=*",
		"--user-data-dir=" + userDataDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--new-window",
		browserLoginURL,
	}
	cmd := exec.Command(browserPath, args...)
	cmd.Env = browserEnvironment(profile)
	if info, statErr := os.Stat(profile); statErr == nil && info.IsDir() {
		cmd.Dir = profile
	}
	if launchToken != 0 {
		cmd.SysProcAttr = &syscall.SysProcAttr{Token: syscall.Token(launchToken)}
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start browser: %w", err)
	}
	cleanup = false

	return &browserLogin{
		Port:        port,
		UserDataDir: userDataDir,
		PID:         cmd.Process.Pid,
		process:     cmd.Process,
	}, nil
}

// PollCookies returns Doubao/ByteDance cookies once the user has signed in.
// A missing sessionid is not an error: it means the sign-in is still pending.
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
	result := cookieMapFromCDP(cookies)
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
			_ = b.process.Kill()
			_ = waitProcessExit(b.PID, time.Second)
		}
		_ = b.process.Release()
		b.process = nil
	} else if b.PID > 0 {
		if process, err := os.FindProcess(b.PID); err == nil {
			if !waitProcessExit(b.PID, 1500*time.Millisecond) {
				_ = process.Kill()
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
	target, ok := pickPageTarget(targets, b.Port)
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

func pickPageTarget(targets []devToolsTarget, port int) (devToolsTarget, bool) {
	var doubaoChat, doubaoAny, fallback *devToolsTarget
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
		if host == "doubao.com" || strings.HasSuffix(host, ".doubao.com") {
			if doubaoAny == nil {
				doubaoAny = target
			}
			if strings.HasPrefix(path, "/chat") && doubaoChat == nil {
				doubaoChat = target
			}
		}
	}
	for _, target := range []*devToolsTarget{doubaoChat, doubaoAny, fallback} {
		if target != nil {
			return *target, true
		}
	}
	return devToolsTarget{}, false
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

// cookieMapFromCDP keeps only Doubao/ByteDance cookies and returns an empty map
// until the session cookie appears.
func cookieMapFromCDP(cookies []cdpCookie) map[string]string {
	result := map[string]string{}
	best := map[string]int{}
	for _, cookie := range cookies {
		name := strings.TrimSpace(cookie.Name)
		if name == "" || strings.TrimSpace(cookie.Value) == "" {
			continue
		}
		if !cookieDomainAllowed(cookie.Domain) {
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

func cookieDomainAllowed(domain string) bool {
	domain = normalizeCookieDomain(domain)
	return domain == "doubao.com" ||
		strings.HasSuffix(domain, ".doubao.com") ||
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

func duplicatePrimaryToken(token windows.Token) (windows.Token, error) {
	var primary windows.Token
	err := windows.DuplicateTokenEx(
		token,
		windows.TOKEN_ASSIGN_PRIMARY|windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY|
			windows.TOKEN_ADJUST_DEFAULT|windows.TOKEN_ADJUST_SESSIONID,
		nil,
		windows.SecurityImpersonation,
		windows.TokenPrimary,
		&primary,
	)
	if err != nil {
		return 0, fmt.Errorf("prepare desktop launch token: %w", err)
	}
	return primary, nil
}

func currentUserProfile() (string, error) {
	if profile := strings.TrimSpace(os.Getenv("USERPROFILE")); profile != "" {
		return filepath.Clean(profile), nil
	}
	profile, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve current user profile: %w", err)
	}
	if strings.TrimSpace(profile) == "" {
		return "", errors.New("current user profile is unavailable")
	}
	return filepath.Clean(profile), nil
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

func newBrowserUserDataDir(profile string) (string, error) {
	root := filepath.Join(profile, "AppData", "Local", "Temp")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("prepare browser temp directory: %w", err)
	}
	dir, err := os.MkdirTemp(root, browserLoginTempPrefix)
	if err != nil {
		return "", fmt.Errorf("create browser profile directory: %w", err)
	}
	return dir, nil
}

func browserEnvironment(profile string) []string {
	if strings.TrimSpace(profile) == "" {
		return os.Environ()
	}
	profile = filepath.Clean(profile)
	localAppData := filepath.Join(profile, "AppData", "Local")
	roamingAppData := filepath.Join(profile, "AppData", "Roaming")
	temp := filepath.Join(localAppData, "Temp")
	overrides := map[string]string{
		"USERPROFILE":  profile,
		"LOCALAPPDATA": localAppData,
		"APPDATA":      roamingAppData,
		"TEMP":         temp,
		"TMP":          temp,
	}
	if volume := filepath.VolumeName(profile); volume != "" {
		homePath := strings.TrimPrefix(profile, volume)
		if homePath == "" {
			homePath = `\`
		}
		overrides["HOMEDRIVE"] = volume
		overrides["HOMEPATH"] = homePath
	}
	return mergeEnvironment(os.Environ(), overrides)
}

func mergeEnvironment(base []string, overrides map[string]string) []string {
	normalized := make(map[string]string, len(overrides))
	for name, value := range overrides {
		normalized[strings.ToUpper(name)] = value
	}

	result := make([]string, 0, len(base)+len(normalized))
	seen := make(map[string]bool, len(base)+len(normalized))
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || name == "" {
			continue
		}
		key := strings.ToUpper(name)
		if _, replaced := normalized[key]; replaced {
			continue
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, entry)
	}

	names := make([]string, 0, len(normalized))
	for name := range normalized {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result = append(result, name+"="+normalized[name])
	}
	return result
}

func resolveBrowserExecutable(profile string) (string, error) {
	if override := strings.TrimSpace(os.Getenv("DOUJIE_BROWSER_PATH")); override != "" {
		override = os.ExpandEnv(strings.Trim(override, `"`))
		if isBrowserExecutable(override) {
			return filepath.Clean(override), nil
		}
		return "", errors.New("DOUJIE_BROWSER_PATH is not an executable file")
	}
	for _, candidate := range browserExecutableCandidates(profile, os.Getenv) {
		if isBrowserExecutable(candidate) {
			return candidate, nil
		}
	}
	return "", errors.New("Microsoft Edge or Google Chrome was not found; install one or set DOUJIE_BROWSER_PATH")
}

func browserExecutableCandidates(profile string, getenv func(string) string) []string {
	if getenv == nil {
		getenv = os.Getenv
	}
	var programDirs []string
	for _, name := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432"} {
		if dir := strings.TrimSpace(getenv(name)); dir != "" {
			programDirs = append(programDirs, dir)
		}
	}
	var localDirs []string
	if dir := strings.TrimSpace(getenv("LOCALAPPDATA")); dir != "" {
		localDirs = append(localDirs, dir)
	}
	if strings.TrimSpace(profile) != "" {
		localDirs = append(localDirs, filepath.Join(profile, "AppData", "Local"))
	}

	var candidates []string
	for _, dir := range programDirs {
		candidates = append(candidates, filepath.Join(dir, "Microsoft", "Edge", "Application", "msedge.exe"))
	}
	for _, dir := range localDirs {
		candidates = append(candidates, filepath.Join(dir, "Microsoft", "Edge", "Application", "msedge.exe"))
	}
	for _, dir := range programDirs {
		candidates = append(candidates, filepath.Join(dir, "Google", "Chrome", "Application", "chrome.exe"))
	}
	for _, dir := range localDirs {
		candidates = append(candidates, filepath.Join(dir, "Google", "Chrome", "Application", "chrome.exe"))
	}

	seen := map[string]bool{}
	unique := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		key := strings.ToLower(candidate)
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, candidate)
	}
	return unique
}

func isBrowserExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func waitProcessExit(pid int, timeout time.Duration) bool {
	if pid <= 0 {
		return true
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return true
	}
	defer windows.CloseHandle(handle)
	milliseconds := uint32(timeout / time.Millisecond)
	if milliseconds == 0 {
		milliseconds = 1
	}
	status, err := windows.WaitForSingleObject(handle, milliseconds)
	return err == nil && status == windows.WAIT_OBJECT_0
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
