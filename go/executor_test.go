package main

import (
	"testing"
	"time"
)

func TestConcurrencyGateAppliesConfigUpdates(t *testing.T) {
	oldConfig := getConfig()
	gateMu.Lock()
	oldGates := gates
	gates = map[string]*concurrencyGate{}
	gateMu.Unlock()
	defer func() {
		configMu.Lock()
		currentConfig = oldConfig
		configMu.Unlock()
		gateMu.Lock()
		gates = oldGates
		gateMu.Unlock()
	}()

	if err := configure([]byte("max_concurrency: 1\n")); err != nil {
		t.Fatal(err)
	}
	releaseFirst := acquire("dynamic")
	waiter := make(chan func(), 1)
	go func() { waiter <- acquire("dynamic") }()
	select {
	case release := <-waiter:
		release()
		t.Fatal("second request acquired above the configured limit")
	case <-time.After(100 * time.Millisecond):
	}

	if err := configure([]byte("max_concurrency: 2\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case release := <-waiter:
		release()
	case <-time.After(time.Second):
		t.Fatal("waiting request did not observe the increased limit")
	}
	releaseFirst()
}
