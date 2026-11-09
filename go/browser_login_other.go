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
	Site        string
	WSURL       string
	client      *cdpClient
	mu          sync.Mutex
}

func startBrowserLogin(site string) (*browserLogin, error) {
	return nil, errors.New("browser login is only supported on Windows")
}

func (b *browserLogin) PollCookies() (map[string]string, error) {
	return nil, errors.New("browser login is only supported on Windows")
}

func (b *browserLogin) Close() {}
