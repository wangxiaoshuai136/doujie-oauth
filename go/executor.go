package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type concurrencyGate struct {
	mu     sync.Mutex
	cond   *sync.Cond
	active int
}

var gateMu sync.Mutex
var gates = map[string]*concurrencyGate{}

func acquire(id string) func() {
	if id == "" {
		id = "default"
	}
	gateMu.Lock()
	gate := gates[id]
	if gate == nil {
		gate = &concurrencyGate{}
		gate.cond = sync.NewCond(&gate.mu)
		gates[id] = gate
	}
	gateMu.Unlock()
	gate.mu.Lock()
	for {
		limit := getConfig().MaxConcurrency
		if limit < 1 {
			limit = 1
		}
		if gate.active < limit {
			break
		}
		gate.cond.Wait()
	}
	gate.active++
	gate.mu.Unlock()
	return func() {
		gate.mu.Lock()
		if gate.active > 0 {
			gate.active--
		}
		gate.cond.Signal()
		gate.mu.Unlock()
	}
}

func broadcastGateUpdates() {
	gateMu.Lock()
	defer gateMu.Unlock()
	for _, gate := range gates {
		gate.mu.Lock()
		gate.cond.Broadcast()
		gate.mu.Unlock()
	}
}

func execute(req pluginapi.ExecutorRequest, sender httpSender) (pluginapi.ExecutorResponse, error) {
	if isVideoModel(req.Model) {
		return executeVideo(req, sender)
	}
	release := acquire(req.AuthID)
	defer release()
	plan, err := buildRequest(req, sender)
	if err != nil {
		return pluginapi.ExecutorResponse{}, err
	}
	resp, err := sender.Do(plan.HTTP)
	if err != nil {
		return pluginapi.ExecutorResponse{}, pluginabi.NewError("transport_error", "Doubao upstream transport failed", 503)
	}
	if resp.StatusCode != http.StatusOK {
		return pluginapi.ExecutorResponse{}, upstreamError(resp.StatusCode, resp.Body)
	}
	var text, think string
	var collectErr error
	if plan.Endpoint == "chat" {
		text, think, collectErr = collectDoubaoChat(resp.Body)
	} else {
		text, think, collectErr = collectDoubao(resp.Body)
	}
	usageEvents.record(req.AuthID, collectErr != nil)
	if collectErr != nil {
		return pluginapi.ExecutorResponse{}, upstreamFailureError(collectErr.Error())
	}
	return pluginapi.ExecutorResponse{
		Payload: encodeMessage(plan.Format, plan.Model, text, think),
		Headers: http.Header{"Content-Type": {"application/json"}},
		Metadata: map[string]any{
			"upstream_host":  "www.doubao.com",
			"upstream_model": plan.Model,
			"deep_think":     plan.DeepThink,
		},
	}, nil
}

func executeStream(req pluginapi.ExecutorRequest, callbackID, streamID string, sender httpSender) (map[string]any, error) {
	if streamID == "" {
		return nil, pluginabi.NewError("invalid_request", "stream ID required", 400)
	}
	if isVideoModel(req.Model) {
		return executeVideoStream(req, callbackID, streamID, sender)
	}
	release := acquire(req.AuthID)
	plan, err := buildRequest(req, sender)
	if err != nil {
		release()
		return nil, err
	}
	response, err := openHostStream(sender, plan.HTTP, callbackID)
	if err != nil {
		release()
		return nil, pluginabi.NewError("transport_error", "Doubao upstream stream failed", 503)
	}
	if response.StreamID == "" {
		release()
		return nil, pluginabi.NewError("transport_error", "Doubao upstream stream unavailable", 503)
	}
	if response.StatusCode != http.StatusOK || strings.Contains(strings.ToLower(response.Headers.Get("Content-Type")), "application/json") {
		raw, readErr := readWholeStream(response.StreamID)
		release()
		if readErr != nil {
			return nil, pluginabi.NewError("transport_error", "Doubao upstream stream failed", 503)
		}
		return nil, upstreamError(response.StatusCode, raw)
	}
	initial, err := bootstrapDoubaoStream(response.StreamID, plan.Endpoint)
	if err != nil {
		closeHostStream(response.StreamID)
		release()
		usageEvents.record(req.AuthID, true)
		return nil, err
	}
	go forwardDoubao(response.StreamID, streamID, req.AuthID, plan.Model, plan.Format, plan.Endpoint, dataFramedChunks(req), initial, release)
	return map[string]any{"headers": http.Header{"Content-Type": {"text/event-stream"}, "Cache-Control": {"no-cache"}}}, nil
}

// bootstrapDoubaoStream waits for the first meaningful upstream event before
// CPA commits an SSE response. This lets quota/auth failures trigger normal
// credential retry instead of becoming a 200 stream that can no longer fail
// over to another account.
func bootstrapDoubaoStream(source, endpoint string) ([]byte, error) {
	var pending []byte
	inThinking := false
	parse := func(payload []byte) (doubaoMessage, bool) {
		if endpoint == "chat" {
			return parseChatEvent(payload, &inThinking)
		}
		return parseDoubaoEvent(payload)
	}
	for {
		chunk, err := readHostStream(source)
		if err != nil {
			return nil, pluginabi.NewError("transport_error", "Doubao upstream stream failed", 503)
		}
		pending = append(pending, chunk.Payload...)
		if len(pending) > 8<<20 {
			return nil, pluginabi.NewError("upstream_error", "Doubao upstream stream too large", 502)
		}
		normalized := bytes.ReplaceAll(pending, []byte("\r\n"), []byte("\n"))
		for {
			index := bytes.Index(normalized, []byte("\n\n"))
			if index < 0 {
				break
			}
			block := string(normalized[:index])
			normalized = normalized[index+2:]
			payload := sseData(block)
			if payload == "" {
				continue
			}
			msg, ok := parse([]byte(payload))
			if !ok {
				continue
			}
			if msg.Failure != "" {
				return nil, upstreamFailureError(msg.Failure)
			}
			if msg.Text != "" || msg.Think != "" || msg.Finished {
				return pending, nil
			}
		}
		if chunk.Done {
			return nil, pluginabi.NewError("upstream_error", "Doubao returned no answer", 502)
		}
	}
}

func forwardDoubao(source, target, authID, model, format, endpoint string, dataOnly bool, initial []byte, release func()) {
	defer release()
	defer closeHostStream(source)
	stream := newStreamEncoder(format, model, dataOnly)
	streamError := ""
	var pending []byte
	inThinking := false
	parse := func(payload []byte) (doubaoMessage, bool) {
		if endpoint == "chat" {
			return parseChatEvent(payload, &inThinking)
		}
		return parseDoubaoEvent(payload)
	}
	emit := func(frames [][]byte) bool {
		for _, frame := range frames {
			if len(frame) == 0 {
				continue
			}
			if err := emitStreamChunk(target, frame); err != nil {
				return false
			}
		}
		return true
	}
	processPending := func() (bool, string) {
		for {
			index := bytes.Index(pending, []byte("\n\n"))
			if index < 0 {
				return false, ""
			}
			block := string(pending[:index])
			pending = pending[index+2:]
			payload := sseData(block)
			if payload == "" {
				continue
			}
			msg, ok := parse([]byte(payload))
			if !ok {
				continue
			}
			if msg.Failure != "" {
				return false, sanitizeUpstream(msg.Failure)
			}
			if !emit(stream.Handle(msg)) {
				return false, "downstream closed"
			}
			if msg.Finished {
				return true, ""
			}
		}
	}
	if !emit(stream.Start()) {
		closeStream(target, "downstream closed")
		return
	}
	pending = bytes.ReplaceAll(append([]byte(nil), initial...), []byte("\r\n"), []byte("\n"))
	finished := false
	if len(pending) > 0 {
		var processErr string
		finished, processErr = processPending()
		if processErr != "" {
			usageEvents.record(authID, true)
			closeStream(target, processErr)
			return
		}
	}
	for !finished {
		chunk, err := readHostStream(source)
		if err != nil {
			usageEvents.record(authID, true)
			closeStream(target, "Doubao upstream stream failed")
			return
		}
		pending = append(pending, chunk.Payload...)
		pending = bytes.ReplaceAll(pending, []byte("\r\n"), []byte("\n"))
		if len(pending) > 8<<20 {
			closeStream(target, "Doubao upstream stream too large")
			return
		}
		var processErr string
		finished, processErr = processPending()
		if processErr != "" {
			usageEvents.record(authID, true)
			closeStream(target, processErr)
			return
		}
		if chunk.Done {
			break
		}
	}
	if payload := sseData(string(pending)); payload != "" {
		if msg, ok := parse([]byte(payload)); ok {
			if msg.Failure != "" {
				streamError = sanitizeUpstream(msg.Failure)
				closeStream(target, streamError)
				return
			}
			if !emit(stream.Handle(msg)) {
				closeStream(target, "downstream closed")
				return
			}
		}
	}
	if streamError != "" {
		usageEvents.record(authID, true)
		closeStream(target, streamError)
		return
	}
	usageEvents.record(authID, false)
	if !emit(stream.Finish()) {
		closeStream(target, "downstream closed")
		return
	}
	closeStream(target, "")
}

func dataFramedChunks(req pluginapi.ExecutorRequest) bool {
	path := requestPath(req)
	if path != "" {
		return !strings.HasPrefix(path, "/v1/messages") && !strings.HasPrefix(path, "/v8/messages")
	}
	source := strings.ToLower(strings.TrimSpace(req.SourceFormat))
	return source != "" && source != "claude"
}

func requestPath(req pluginapi.ExecutorRequest) string {
	path, _ := req.Metadata["request_path"].(string)
	return strings.ToLower(strings.TrimSpace(path))
}

func upstreamError(status int, body []byte) error {
	message := upstreamMessage(body)
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return pluginabi.NewError("reauth_required", "Doubao session expired or rejected; sign in again", 401)
	case status == http.StatusTooManyRequests:
		return quotaFailureError(message)
	case quotaExhaustedMessage(message):
		return quotaFailureError(message)
	case status >= 500:
		return pluginabi.NewError("upstream_error", "Doubao upstream failed: "+message, 502)
	case status == 0:
		return pluginabi.NewError("transport_error", "Doubao upstream transport failed", 503)
	default:
		return pluginabi.NewError("upstream_rejected", "Doubao rejected the request: "+message, status)
	}
}

func upstreamFailureError(message string) error {
	message = sanitizeUpstream(message)
	if quotaExhaustedMessage(message) {
		return quotaFailureError(message)
	}
	return pluginabi.NewError("upstream_error", "Doubao upstream failed: "+message, 502)
}

func quotaFailureError(message string) error {
	return pluginabi.NewError("quota_exhausted", "Doubao rate limit or subscription quota reached: "+sanitizeUpstream(message), 429)
}

func quotaExhaustedMessage(message string) bool {
	text := strings.ToLower(strings.TrimSpace(message))
	if text == "" {
		return false
	}
	for _, marker := range []string{
		"insufficient_quota",
		"quota exceeded",
		"quota exhausted",
		"usage limit",
		"rate limit",
		"too many requests",
		"额度不足",
		"额度已用完",
		"额度已耗尽",
		"额度耗尽",
		"配额不足",
		"配额已用完",
		"配额耗尽",
		"次数已用完",
		"请求次数已达上限",
		"使用次数已达上限",
		"免费次数已用完",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func upstreamMessage(body []byte) string {
	var envelope struct {
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
		Msg     string          `json:"msg"`
		Data    json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &envelope) == nil {
		if envelope.Msg != "" {
			return sanitizeUpstream(envelope.Msg)
		}
		if envelope.Message != "" {
			return sanitizeUpstream(envelope.Message)
		}
	}
	return sanitizeUpstream(string(body))
}
