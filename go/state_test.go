package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSealOpenStateRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	id := strings.Repeat("a", 32)
	expires := time.Unix(1900000000, 0)
	sealed, err := sealState(key, id, expires)
	if err != nil {
		t.Fatalf("sealState: %v", err)
	}
	gotID, gotExpires, err := openState(key, sealed)
	if err != nil {
		t.Fatalf("openState: %v", err)
	}
	if gotID != id || !gotExpires.Equal(expires) {
		t.Fatalf("round trip mismatch: %q %v", gotID, gotExpires)
	}
}

func TestOpenStateRejectsTampering(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	sealed, err := sealState(key, strings.Repeat("b", 32), time.Unix(1900000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	other := bytes.Repeat([]byte{8}, 32)
	if _, _, err := openState(other, sealed); err == nil {
		t.Fatal("expected tampered state to fail")
	}
	if _, _, err := openState(key, "not-a-state"); err == nil {
		t.Fatal("expected malformed state to fail")
	}
}
