package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
)

const (
	doubaoVerificationCode = 710022004
	doubaoRateLimitCode    = 710022002
	doubaoVideoQuotaCode   = 710012001
)

// doubaoStreamError keeps the upstream error code and risk-control decision
// that the plain SSE message loses. The chat endpoint can return HTTP 200 and
// then fail the stream with error_code 710022004 plus a verify decision.
type doubaoStreamError struct {
	Code    int
	Message string
	Scene   string
	Subtype string
}

func (e *doubaoStreamError) Error() string {
	if e == nil {
		return ""
	}
	message := strings.TrimSpace(e.Message)
	if message == "" {
		return fmt.Sprintf("code=%d", e.Code)
	}
	return fmt.Sprintf("code=%d %s", e.Code, message)
}

type doubaoDecision struct {
	From        string `json:"from"`
	Type        string `json:"type"`
	Subtype     string `json:"subtype"`
	VerifyScene string `json:"verify_scene"`
}

func newDoubaoStreamError(code int, message string, source map[string]any) *doubaoStreamError {
	if code == 0 {
		return nil
	}
	streamErr := &doubaoStreamError{Code: code, Message: sanitizeUpstream(strings.TrimSpace(message))}
	decision := decisionFromMap(source)
	if strings.EqualFold(decision.Type, "verify") {
		streamErr.Scene = strings.TrimSpace(decision.VerifyScene)
		streamErr.Subtype = strings.TrimSpace(decision.Subtype)
	}
	return streamErr
}

func decisionFromMap(obj map[string]any) doubaoDecision {
	if len(obj) == 0 {
		return doubaoDecision{}
	}
	if decision, ok := decodeDoubaoDecision(obj["decision"]); ok {
		return decision
	}
	for _, key := range []string{"extra", "data"} {
		nested := mapValue(obj[key])
		if len(nested) == 0 {
			continue
		}
		if decision, ok := decodeDoubaoDecision(nested["decision"]); ok {
			return decision
		}
	}
	return doubaoDecision{}
}

func decodeDoubaoDecision(value any) (doubaoDecision, bool) {
	switch item := value.(type) {
	case string:
		item = strings.TrimSpace(item)
		if item == "" {
			return doubaoDecision{}, false
		}
		var decision doubaoDecision
		if json.Unmarshal([]byte(item), &decision) == nil {
			return decision, true
		}
	case map[string]any:
		raw, err := json.Marshal(item)
		if err != nil {
			return doubaoDecision{}, false
		}
		var decision doubaoDecision
		if json.Unmarshal(raw, &decision) == nil {
			return decision, true
		}
	}
	return doubaoDecision{}, false
}

func looseInt(value any) int {
	if n := asInt(value); n != 0 {
		return n
	}
	if text, ok := value.(string); ok {
		n, _ := strconv.Atoi(strings.TrimSpace(text))
		return n
	}
	return 0
}

func firstStringValue(values ...any) string {
	for _, value := range values {
		switch item := value.(type) {
		case string:
			if text := strings.TrimSpace(item); text != "" {
				return text
			}
		case nil:
		default:
			if text := strings.TrimSpace(fmt.Sprintf("%v", item)); text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

func knownDoubaoStreamCode(code int) bool {
	switch code {
	case doubaoVerificationCode, doubaoRateLimitCode, doubaoVideoQuotaCode:
		return true
	default:
		return false
	}
}

func streamErrorFromObject(obj map[string]any) *doubaoStreamError {
	if len(obj) == 0 {
		return nil
	}
	code := looseInt(obj["error_code"])
	decision := decisionFromMap(obj)
	if code == 0 && strings.EqualFold(decision.Type, "verify") {
		code = doubaoVerificationCode
	}
	if !knownDoubaoStreamCode(code) {
		return nil
	}
	message := firstStringValue(obj["error_msg"], obj["message"], obj["msg"])
	return newDoubaoStreamError(code, message, obj)
}

func streamErrorFromBody(body []byte) *doubaoStreamError {
	var obj map[string]any
	if json.Unmarshal(body, &obj) != nil {
		return nil
	}
	if streamErr := streamErrorFromObject(obj); streamErr != nil {
		return streamErr
	}
	for _, key := range []string{"error", "data"} {
		if streamErr := streamErrorFromObject(mapValue(obj[key])); streamErr != nil {
			return streamErr
		}
	}
	return nil
}

func objectFromRaw(raw json.RawMessage) map[string]any {
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil {
		return obj
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return mapValue(text)
	}
	return nil
}

func applyStreamErrorCooldown(authID string, streamErr *doubaoStreamError) {
	if streamErr == nil {
		return
	}
	switch streamErr.Code {
	case doubaoVerificationCode:
		setAuthCooldown(authID, verificationCooldown, "verification", streamErr.Scene)
	case doubaoRateLimitCode:
		setAuthCooldown(authID, rateLimitCooldown, "rate_limit", "")
	}
}

func classifyUpstreamFailure(authID string, err error) error {
	if err == nil {
		return nil
	}
	var streamErr *doubaoStreamError
	if errors.As(err, &streamErr) && streamErr != nil {
		applyStreamErrorCooldown(authID, streamErr)
		switch streamErr.Code {
		case doubaoVerificationCode:
			return verificationRequiredError(streamErr)
		case doubaoRateLimitCode:
			return rateLimitedError(streamErr)
		case doubaoVideoQuotaCode:
			return quotaFailureError(streamErr.Error())
		default:
			return upstreamFailureError(streamErr.Error())
		}
	}
	return upstreamFailureError(err.Error())
}

func verificationRequiredError(streamErr *doubaoStreamError) error {
	scene := strings.TrimSpace(streamErr.Scene)
	if scene == "" {
		scene = "doubao_message_web"
	}
	detail := fmt.Sprintf("Doubao risk verification required (scene=%s", scene)
	if subtype := strings.TrimSpace(streamErr.Subtype); subtype != "" {
		detail += ", subtype=" + subtype
	}
	detail += "); complete the verification in Doubao, then retry"
	err := pluginabi.NewError("verification_required", detail, http.StatusTooManyRequests)
	err.RetryAfterSeconds = int(verificationCooldown.Seconds())
	err.CredentialScoped = true
	return err
}

func rateLimitedError(streamErr *doubaoStreamError) error {
	err := pluginabi.NewError(
		"rate_limited",
		fmt.Sprintf("Doubao temporarily rate limited this account: %s", sanitizeUpstream(streamErr.Error())),
		http.StatusTooManyRequests,
	)
	err.RetryAfterSeconds = int(rateLimitCooldown.Seconds())
	err.CredentialScoped = true
	return err
}

func streamFailureText(streamErr *doubaoStreamError) string {
	if streamErr == nil {
		return ""
	}
	if streamErr.Code == doubaoVerificationCode {
		scene := strings.TrimSpace(streamErr.Scene)
		if scene == "" {
			scene = "doubao_message_web"
		}
		return fmt.Sprintf("Doubao risk verification required (scene=%s); complete the verification in Doubao and retry", scene)
	}
	return sanitizeUpstream(streamErr.Error())
}
