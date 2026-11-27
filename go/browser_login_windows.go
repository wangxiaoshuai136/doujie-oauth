//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func startBrowserLoginWithBrowser(site, browser string) (*browserLogin, error) {
	site = normalizeSite(site)
	port, err := freeLoopbackPort()
	if err != nil {
		return nil, err
	}

	var launchToken windows.Token
	var profile string
	var browserPath string
	if processRunsInSessionZero() {
		consoleToken, consoleProfile, err := activeConsoleUser()
		if err != nil {
			return nil, fmt.Errorf("locate the active desktop user: %w", err)
		}
		profile = consoleProfile
		browserPath, err = resolveBrowserExecutableForUser(profile, consoleToken, browser)
		if err != nil {
			consoleToken.Close()
			return nil, fmt.Errorf("resolve the desktop user's default browser: %w", err)
		}
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
		browserPath, err = resolveBrowserExecutableNamed(profile, browser)
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

	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-allow-origins=*",
		"--user-data-dir=" + userDataDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--new-window",
		profileForSite(site).Root + "/chat/",
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
		Site:        site,
		process:     cmd.Process,
	}, nil
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

func resolveBrowserExecutableForUser(profile string, token windows.Token, browser string) (string, error) {
	if token == 0 {
		return resolveBrowserExecutableNamed(profile, browser)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ok, _, callErr := procImpersonateUser.Call(uintptr(token))
	if ok == 0 {
		return "", fmt.Errorf("ImpersonateLoggedOnUser failed: %v", callErr)
	}
	defer procRevertToSelf.Call()
	return resolveBrowserExecutableNamed(profile, browser)
}

func resolveBrowserExecutable(profile string) (string, error) {
	return resolveBrowserExecutableNamed(profile, "auto")
}

func resolveBrowserExecutableNamed(profile, browser string) (string, error) {
	if override := strings.TrimSpace(os.Getenv("DOUJIE_BROWSER_PATH")); override != "" {
		override = os.ExpandEnv(strings.Trim(override, `"`))
		if isBrowserExecutable(override) {
			return filepath.Clean(override), nil
		}
		return "", errors.New("DOUJIE_BROWSER_PATH is not an executable file")
	}
	preference := strings.ToLower(strings.TrimSpace(browser))
	switch preference {
	case "", "auto", "default":
	case "chrome", "google chrome":
		for _, candidate := range browserExecutableCandidates(profile, os.Getenv) {
			if isBrowserExecutable(candidate) {
				return candidate, nil
			}
		}
		return "", errors.New("Google Chrome is not installed")
	case "edge", "microsoft edge":
		for _, candidate := range edgeExecutableCandidates(profile, os.Getenv) {
			if isBrowserExecutable(candidate) {
				return candidate, nil
			}
		}
		return "", errors.New("Microsoft Edge is not installed")
	default:
		path := filepath.Clean(os.ExpandEnv(strings.Trim(preference, `"`)))
		if isBrowserExecutable(path) {
			return path, nil
		}
		return "", errors.New("configured browser path is not an executable file")
	}
	if defaultPath, defaultErr := defaultBrowserExecutable(); defaultErr == nil && isChromiumBrowser(defaultPath) && isBrowserExecutable(defaultPath) {
		return defaultPath, nil
	}
	for _, candidate := range browserExecutableCandidates(profile, os.Getenv) {
		if isBrowserExecutable(candidate) {
			return candidate, nil
		}
	}
	return "", errors.New("the default browser is not Chromium-compatible; use Chrome or set DOUJIE_BROWSER_PATH")
}

func defaultBrowserExecutable() (string, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\Shell\Associations\UrlAssociations\http\UserChoice`, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	progID, _, err := key.GetStringValue("ProgId")
	_ = key.Close()
	if err != nil || strings.TrimSpace(progID) == "" {
		return "", errors.New("default browser association is unavailable")
	}
	commandKey, err := registry.OpenKey(registry.CLASSES_ROOT, progID+`\shell\open\command`, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer commandKey.Close()
	command, _, err := commandKey.GetStringValue("")
	if err != nil {
		return "", err
	}
	return parseBrowserCommand(command)
}

func parseBrowserCommand(command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", errors.New("default browser command is empty")
	}
	if command[0] == '"' {
		end := strings.IndexByte(command[1:], '"')
		if end < 0 {
			return "", errors.New("default browser command has an unterminated path")
		}
		return filepath.Clean(os.ExpandEnv(command[1 : end+1])), nil
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "", errors.New("default browser executable is missing")
	}
	return filepath.Clean(os.ExpandEnv(fields[0])), nil
}

func isChromiumBrowser(path string) bool {
	switch strings.ToLower(filepath.Base(strings.TrimSpace(path))) {
	case "chrome.exe", "msedge.exe", "brave.exe", "vivaldi.exe", "chromium.exe", "thorium.exe":
		return true
	default:
		return false
	}
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

func edgeExecutableCandidates(profile string, getenv func(string) string) []string {
	if getenv == nil {
		getenv = os.Getenv
	}
	var roots []string
	for _, name := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432"} {
		if dir := strings.TrimSpace(getenv(name)); dir != "" {
			roots = append(roots, filepath.Join(dir, "Microsoft", "Edge", "Application", "msedge.exe"))
		}
	}
	if dir := strings.TrimSpace(getenv("LOCALAPPDATA")); dir != "" {
		roots = append(roots, filepath.Join(dir, "Microsoft", "Edge", "Application", "msedge.exe"))
	}
	if strings.TrimSpace(profile) != "" {
		roots = append(roots, filepath.Join(profile, "AppData", "Local", "Microsoft", "Edge", "Application", "msedge.exe"))
	}
	seen := map[string]bool{}
	unique := make([]string, 0, len(roots))
	for _, candidate := range roots {
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

func killBrowserProcess(process *os.Process, pid int) {
	if process != nil {
		_ = process.Kill()
	}
}
