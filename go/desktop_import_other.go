//go:build !windows

package main

import "errors"

func readDesktopCookies() (map[string]string, error) {
	return nil, errors.New("desktop import is only supported on Windows")
}
