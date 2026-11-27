package main

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
)

const (
	verificationCooldown = 10 * time.Minute
	rateLimitCooldown    = 30 * time.Second
)

type authCooldownState struct {
	Until  time.Time
	Reason string
	Scene  string
}

var authCooldownMu sync.Mutex
var authCooldowns = map[string]authCooldownState{}

func authCooldownKey(authID string) string {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return "default"
	}
	return authID
}

func setAuthCooldown(authID string, d time.Duration, reason, scene string) {
	if d <= 0 {
		return
	}
	key := authCooldownKey(authID)
	until := time.Now().Add(d)
	authCooldownMu.Lock()
	defer authCooldownMu.Unlock()
	if existing, ok := authCooldowns[key]; ok && existing.Until.After(until) {
		return
	}
	authCooldowns[key] = authCooldownState{Until: until, Reason: reason, Scene: scene}
}

func authCooldownError(authID string) error {
	key := authCooldownKey(authID)
	now := time.Now()
	authCooldownMu.Lock()
	state, ok := authCooldowns[key]
	if !ok {
		authCooldownMu.Unlock()
		return nil
	}
	if !state.Until.After(now) {
		delete(authCooldowns, key)
		authCooldownMu.Unlock()
		return nil
	}
	remaining := int(state.Until.Sub(now).Seconds())
	if remaining < 1 {
		remaining = 1
	}
	authCooldownMu.Unlock()

	if state.Reason == "verification" {
		scene := strings.TrimSpace(state.Scene)
		if scene == "" {
			scene = "doubao_message_web"
		}
		err := pluginabi.NewError(
			"verification_required",
			fmt.Sprintf("Doubao risk verification required (scene=%s); complete the verification in Doubao, then retry", scene),
			http.StatusTooManyRequests,
		)
		err.RetryAfterSeconds = remaining
		err.CredentialScoped = true
		return err
	}
	err := pluginabi.NewError(
		"rate_limited",
		fmt.Sprintf("Doubao temporarily rate limited this account; retry in %d seconds", remaining),
		http.StatusTooManyRequests,
	)
	err.RetryAfterSeconds = remaining
	err.CredentialScoped = true
	return err
}
