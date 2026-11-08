package main

import "testing"

func TestConfigureDefaults(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	cfg := getConfig()
	if cfg.MaxConcurrency != 3 || cfg.BotID != defaultBotID || cfg.Signer != "none" {
		t.Fatalf("unexpected defaults: %+v", cfg)
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
}
