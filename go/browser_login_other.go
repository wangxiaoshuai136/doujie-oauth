//go:build !windows

package main

import (
	"errors"
	"sync"
)

type cdpClient struct{}

type browserLogin struct {
	Port        int
	UserDataDir string
	PID         int
	WSURL       string
	client      *cdpClient
	mu          sync.Mutex
}

func startBrowserLogin() (*browserLogin, error) {
	return nil, errors.New("browser login is only supported on Windows")
}

func (b *browserLogin) PollCookies() (map[string]string, error) {
	return nil, errors.New("browser login is only supported on Windows")
}

func (b *browserLogin) Close() {}
