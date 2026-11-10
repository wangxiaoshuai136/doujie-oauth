package main

import (
	"fmt"
	"net/url"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type config struct {
	MaxConcurrency int    `yaml:"max_concurrency"`
	BotID          string `yaml:"bot_id"`
	DeepThink      bool   `yaml:"deep_think"`
	Signer         string `yaml:"signer"`
	SignerURL      string `yaml:"signer_url"`
	PublicBaseURL  string `yaml:"public_base_url"`
	AppVersion     string `yaml:"app_version"`
	Endpoint       string `yaml:"endpoint"`
}

var configMu sync.RWMutex
var currentConfig = defaults()

func defaults() config {
	return config{
		MaxConcurrency: 3,
		BotID:          defaultBotID,
		DeepThink:      false,
		Signer:         "none",
		AppVersion:     "2.1.7",
		Endpoint:       "samantha",
	}
}

func getConfig() config {
	configMu.RLock()
	defer configMu.RUnlock()
	return currentConfig
}

func configure(raw []byte) error {
	c := defaults()
	if len(raw) > 0 {
		if err := yaml.Unmarshal(raw, &c); err != nil {
			return fmt.Errorf("invalid plugin configuration")
		}
	}
	c.Signer = strings.ToLower(strings.TrimSpace(c.Signer))
	if c.Signer == "" {
		c.Signer = "none"
	}
	if c.Signer != "none" && c.Signer != "external" {
		return fmt.Errorf("invalid doubao plugin configuration: signer must be none or external")
	}
	c.SignerURL = strings.TrimSpace(c.SignerURL)
	if c.Signer == "external" {
		if !trustedSignerURL(c.SignerURL) {
			return fmt.Errorf("invalid doubao plugin configuration: external signer requires HTTPS, or plain HTTP on loopback")
		}
	}
	c.PublicBaseURL = strings.TrimSpace(c.PublicBaseURL)
	if c.PublicBaseURL != "" {
		if _, ok := httpBaseURL(c.PublicBaseURL); !ok {
			return fmt.Errorf("invalid doubao plugin configuration: public_base_url must be an absolute HTTP(S) URL")
		}
	}
	c.BotID = strings.TrimSpace(c.BotID)
	if c.BotID == "" {
		return fmt.Errorf("invalid doubao plugin configuration: bot_id must not be empty")
	}
	if c.MaxConcurrency < 1 || c.MaxConcurrency > 100 {
		return fmt.Errorf("invalid doubao plugin configuration: max_concurrency must be 1-100")
	}
	c.Endpoint = strings.ToLower(strings.TrimSpace(c.Endpoint))
	if c.Endpoint == "" {
		c.Endpoint = "samantha"
	}
	if c.Endpoint != "samantha" && c.Endpoint != "chat" {
		return fmt.Errorf("invalid doubao plugin configuration: endpoint must be samantha or chat")
	}
	c.AppVersion = strings.TrimSpace(c.AppVersion)
	if c.AppVersion == "" {
		return fmt.Errorf("invalid doubao plugin configuration: app_version must not be empty")
	}
	configMu.Lock()
	currentConfig = c
	configMu.Unlock()
	broadcastGateUpdates()
	return nil
}

func httpBaseURL(raw string) (*url.URL, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, false
	}
	return u, true
}
