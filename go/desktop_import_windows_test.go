//go:build windows

package main

import (
	"crypto/sha256"
	"testing"
)

func TestCookiePlaintextStripsChromiumHostBinding(t *testing.T) {
	hostKey := "www.doubao.com"
	value := "session-cookie-value"
	prefix := sha256.Sum256([]byte(hostKey))
	raw := append(append([]byte(nil), prefix[:]...), []byte(value)...)
	if got := cookiePlaintext(hostKey, raw); got != value {
		t.Fatalf("cookiePlaintext = %q, want %q", got, value)
	}
}

func TestCookiePlaintextStripsLegacyBinaryPrefix(t *testing.T) {
	raw := append([]byte{0, 0, 0, 0}, []byte("ttwid-value")...)
	if got := cookiePlaintext("unbound-host", raw); got != "ttwid-value" {
		t.Fatalf("cookiePlaintext legacy = %q", got)
	}
}
