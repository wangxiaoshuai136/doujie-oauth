package main

import "testing"

func TestConfigureDefaults(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	cfg := getConfig()
	if cfg.MaxConcurrency != 3 || cfg.BotID != defaultBotID || cfg.Signer != "none" || !cfg.AutoOpenBrowser || cfg.Browser != "auto" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestAutoOpenBrowserPreference(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = configure(nil) }()
	if !autoOpenBrowser(nil) {
		t.Fatal("default must auto-open")
	}
	if autoOpenBrowser(map[string]any{"auto_open": "false"}) {
		t.Fatal("metadata false must disable auto-open")
	}
	if autoOpenBrowser(map[string]any{"auto_open": "true"}) == false {
		t.Fatal("metadata true must enable auto-open")
	}
	if browserPreference(map[string]any{"browser": "edge"}) != "edge" {
		t.Fatal("metadata browser must win")
	}
	if browserPreference(nil) != "auto" {
		t.Fatal("default browser preference must be auto")
	}
}

func TestConfigureValidation(t *testing.T) {
	cases := []string{
		"max_concurrency: 0",
		"max_concurrency: 101",
		"signer: magic",
		"signer: external",
		"bot_id: ''",
		"public_base_url: not-a-url",
	}
	for _, raw := range cases {
		if err := configure([]byte(raw)); err == nil {
			t.Fatalf("expected configuration %q to fail", raw)
		}
	}
	if err := configure([]byte("signer: external\nsigner_url: http://127.0.0.1:8787/sign")); err != nil {
		t.Fatalf("external signer should be accepted: %v", err)
	}
	if err := configure([]byte("signer: external\nsigner_url: http://signer.example.com/sign")); err == nil {
		t.Fatal("remote plaintext signer must be rejected")
	}
	if err := configure([]byte("signer: external\nsigner_url: https://signer.example.com/sign")); err != nil {
		t.Fatalf("remote HTTPS signer should be accepted: %v", err)
	}
	if err := configure([]byte("browser: not-a-browser")); err == nil {
		t.Fatal("unknown browser name must be rejected")
	}
	if err := configure([]byte("browser: edge")); err != nil {
		t.Fatalf("edge browser should be accepted: %v", err)
	}
}
