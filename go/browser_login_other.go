//go:build !windows && !darwin

package main

import "errors"

func startBrowserLoginWithBrowser(site, browser string) (*browserLogin, error) {
	return nil, errors.New("browser login is only supported on Windows and macOS")
}
