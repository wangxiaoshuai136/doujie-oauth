//go:build darwin

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// startBrowserLoginWithBrowser launches a Chromium-compatible browser with an
// isolated profile and a loopback-only DevTools endpoint. macOS has no session
// 0 / token handoff, so the process inherits the current desktop user.
func startBrowserLoginWithBrowser(site, browser string) (*browserLogin, error) {
	site = normalizeSite(site)
	port, err := freeLoopbackPort()
	if err != nil {
		return nil, err
	}
	profile, err := currentUserProfile()
	if err != nil {
		return nil, err
	}
	browserPath, err := resolveBrowserExecutableNamed(profile, browser)
	if err != nil {
		return nil, err
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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

func currentUserProfile() (string, error) {
	if profile := strings.TrimSpace(os.Getenv("HOME")); profile != "" {
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
	dir, err := os.MkdirTemp("", browserLoginTempPrefix)
	if err != nil {
		return "", fmt.Errorf("create browser profile directory: %w", err)
	}
	return dir, nil
}

func browserEnvironment(profile string) []string {
	return os.Environ()
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
		for _, candidate := range browserExecutableCandidates(profile) {
			if isBrowserExecutable(candidate) {
				return candidate, nil
			}
		}
		return "", errors.New("no Chromium-compatible browser found; install Google Chrome or Microsoft Edge, or set DOUJIE_BROWSER_PATH")
	case "chrome", "google chrome":
		for _, candidate := range chromeExecutableCandidates(profile) {
			if isBrowserExecutable(candidate) {
				return candidate, nil
			}
		}
		return "", errors.New("Google Chrome is not installed")
	case "edge", "microsoft edge":
		for _, candidate := range edgeExecutableCandidates(profile) {
			if isBrowserExecutable(candidate) {
				return candidate, nil
			}
		}
		return "", errors.New("Microsoft Edge is not installed")
	case "safari", "firefox", "mozilla firefox":
		return "", errors.New("only Chromium-compatible browsers are supported; install Google Chrome or Microsoft Edge")
	default:
		path := filepath.Clean(os.ExpandEnv(strings.Trim(preference, `"`)))
		if isBrowserExecutable(path) {
			return path, nil
		}
		return "", errors.New("configured browser path is not an executable file")
	}
}

func chromeExecutableCandidates(profile string) []string {
	return dedupeBrowserPaths([]string{
		filepath.Join("/Applications", "Google Chrome.app", "Contents", "MacOS", "Google Chrome"),
		filepath.Join(profile, "Applications", "Google Chrome.app", "Contents", "MacOS", "Google Chrome"),
	})
}

func edgeExecutableCandidates(profile string) []string {
	return dedupeBrowserPaths([]string{
		filepath.Join("/Applications", "Microsoft Edge.app", "Contents", "MacOS", "Microsoft Edge"),
		filepath.Join(profile, "Applications", "Microsoft Edge.app", "Contents", "MacOS", "Microsoft Edge"),
	})
}

func browserExecutableCandidates(profile string) []string {
	candidates := append([]string{}, chromeExecutableCandidates(profile)...)
	candidates = append(candidates, edgeExecutableCandidates(profile)...)
	for _, name := range []string{"Brave Browser", "Chromium", "Vivaldi", "Thorium"} {
		candidates = append(candidates,
			filepath.Join("/Applications", name+".app", "Contents", "MacOS", name),
			filepath.Join(profile, "Applications", name+".app", "Contents", "MacOS", name),
		)
	}
	return dedupeBrowserPaths(candidates)
}

func dedupeBrowserPaths(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}

func isChromiumBrowser(path string) bool {
	switch strings.ToLower(filepath.Base(strings.TrimSpace(path))) {
	case "google chrome", "microsoft edge", "brave browser", "chromium", "vivaldi", "thorium":
		return true
	default:
		return false
	}
}

func isBrowserExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}
