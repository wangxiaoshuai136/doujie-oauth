package main

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
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

func TestModelUnavailableErrorClassification(t *testing.T) {
	for _, message := range []string{
		"model not available for this account",
		"unsupported model",
		"该模型未开通",
	} {
		err := upstreamError(http.StatusBadRequest, []byte(`{"message":"`+message+`"}`))
		typed, ok := err.(*pluginabi.Error)
		if !ok || typed.Code != "model_unavailable" || typed.HTTPStatus != http.StatusBadRequest || typed.Retryable {
			t.Fatalf("unexpected model error for %q: %#v", message, err)
		}
	}
	err := upstreamFailureError("model_not_found")
	typed, ok := err.(*pluginabi.Error)
	if !ok || typed.Code != "model_unavailable" {
		t.Fatalf("stream model error not classified: %#v", err)
	}
}

func TestQuotaFailureClassification(t *testing.T) {
	for _, message := range []string{
		"insufficient_quota",
		"usage limit reached",
		"Too Many Requests",
		"当前账号额度已用完",
		"请求次数已达上限，请稍后再试",
	} {
		err := upstreamFailureError(message)
		var pluginErr *pluginabi.Error
		if !errors.As(err, &pluginErr) || pluginErr.StatusCode() != http.StatusTooManyRequests {
			t.Fatalf("quota message %q mapped to %#v (%v)", message, pluginErr, err)
		}
	}

	err := upstreamFailureError("temporary upstream failure")
	var pluginErr *pluginabi.Error
	if !errors.As(err, &pluginErr) || pluginErr.StatusCode() != http.StatusBadGateway {
		t.Fatalf("non-quota failure mapped to %#v (%v)", pluginErr, err)
	}
}

func TestUpstreamErrorDetectsQuotaBodyOn500(t *testing.T) {
	err := upstreamError(http.StatusInternalServerError, []byte(`{"message":"本月额度已耗尽"}`))
	var pluginErr *pluginabi.Error
	if !errors.As(err, &pluginErr) || pluginErr.StatusCode() != http.StatusTooManyRequests {
		t.Fatalf("quota body mapped to %#v (%v)", pluginErr, err)
	}
}

func resetAuthCooldownsForTest() {
	authCooldownMu.Lock()
	authCooldowns = map[string]authCooldownState{}
	authCooldownMu.Unlock()
}

func TestVerificationChallengeClassificationAndCooldown(t *testing.T) {
	resetAuthCooldownsForTest()
	defer resetAuthCooldownsForTest()

	streamErr := &doubaoStreamError{
		Code:    doubaoVerificationCode,
		Message: "rate limited",
		Scene:   "doubao_message_web",
		Subtype: "semantic_reasoning",
	}
	err := classifyUpstreamFailure("verify-auth", streamErr)
	var pluginErr *pluginabi.Error
	if !errors.As(err, &pluginErr) {
		t.Fatalf("expected plugin error, got %#v", err)
	}
	if pluginErr.Code != "verification_required" || pluginErr.HTTPStatus != http.StatusTooManyRequests {
		t.Fatalf("unexpected classification: %#v", pluginErr)
	}
	if pluginErr.RetryAfterSeconds != int(verificationCooldown.Seconds()) || !pluginErr.CredentialScoped {
		t.Fatalf("missing retry guidance or credential scope: %#v", pluginErr)
	}

	blocked := authCooldownError("verify-auth")
	var cooldownErr *pluginabi.Error
	if !errors.As(blocked, &cooldownErr) || cooldownErr.Code != "verification_required" {
		t.Fatalf("verification cooldown not applied: %#v", blocked)
	}
	if cooldownErr.RetryAfterSeconds < 1 || cooldownErr.RetryAfterSeconds > int(verificationCooldown.Seconds()) {
		t.Fatalf("unexpected cooldown retry window: %d", cooldownErr.RetryAfterSeconds)
	}
	if authCooldownError("other-auth") != nil {
		t.Fatal("cooldown leaked to another auth")
	}
}

func TestRateLimitClassificationAndCooldown(t *testing.T) {
	resetAuthCooldownsForTest()
	defer resetAuthCooldownsForTest()

	err := classifyUpstreamFailure("rate-auth", &doubaoStreamError{Code: doubaoRateLimitCode, Message: "rate limited"})
	var pluginErr *pluginabi.Error
	if !errors.As(err, &pluginErr) || pluginErr.Code != "rate_limited" || pluginErr.HTTPStatus != http.StatusTooManyRequests {
		t.Fatalf("unexpected rate-limit classification: %#v", err)
	}
	if pluginErr.RetryAfterSeconds != int(rateLimitCooldown.Seconds()) || !pluginErr.CredentialScoped {
		t.Fatalf("missing rate-limit guidance: %#v", pluginErr)
	}
	if authCooldownError("rate-auth") == nil {
		t.Fatal("rate-limit cooldown not applied")
	}
}

func TestAuthCooldownExpires(t *testing.T) {
	resetAuthCooldownsForTest()
	defer resetAuthCooldownsForTest()

	authCooldownMu.Lock()
	authCooldowns["expired"] = authCooldownState{Until: time.Now().Add(-time.Second), Reason: "verification"}
	authCooldownMu.Unlock()
	if err := authCooldownError("expired"); err != nil {
		t.Fatalf("expired cooldown still blocks: %v", err)
	}
	authCooldownMu.Lock()
	_, exists := authCooldowns["expired"]
	authCooldownMu.Unlock()
	if exists {
		t.Fatal("expired cooldown was not cleaned up")
	}
}

func TestRateLimitCooldownDoesNotShortenVerification(t *testing.T) {
	resetAuthCooldownsForTest()
	defer resetAuthCooldownsForTest()

	setAuthCooldown("keep-verify", verificationCooldown, "verification", "doubao_message_web")
	setAuthCooldown("keep-verify", rateLimitCooldown, "rate_limit", "")

	err := authCooldownError("keep-verify")
	var pluginErr *pluginabi.Error
	if !errors.As(err, &pluginErr) || pluginErr.Code != "verification_required" {
		t.Fatalf("shorter rate-limit cooldown replaced verification: %#v", err)
	}
}

func TestUpstreamErrorForVerificationBody(t *testing.T) {
	resetAuthCooldownsForTest()
	defer resetAuthCooldownsForTest()

	body := []byte(`{"error_code":710022004,"error_msg":"rate limited","extra":{"decision":"{\"type\":\"verify\",\"verify_scene\":\"doubao_message_web\"}"}}`)
	err := upstreamErrorFor("http-verify", http.StatusTooManyRequests, body)
	var pluginErr *pluginabi.Error
	if !errors.As(err, &pluginErr) || pluginErr.Code != "verification_required" {
		t.Fatalf("structured HTTP body not classified: %#v", err)
	}
	if authCooldownError("http-verify") == nil {
		t.Fatal("HTTP verification body did not apply cooldown")
	}
}
