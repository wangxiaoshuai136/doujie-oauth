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
		return pluginapi.ExecutorResponse{}, pluginabi.NewError("upstream_error", collectErr.Error(), 502)
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
	go forwardDoubao(response.StreamID, streamID, req.AuthID, plan.Model, plan.Format, plan.Endpoint, dataFramedChunks(req), release)
	return map[string]any{"headers": http.Header{"Content-Type": {"text/event-stream"}, "Cache-Control": {"no-cache"}}}, nil
}

func forwardDoubao(source, target, authID, model, format, endpoint string, dataOnly bool, release func()) {
	defer release()
	defer closeHostStream(source)
	stream := newStreamEncoder(format, model, dataOnly)
	streamError := ""
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
	if !emit(stream.Start()) {
		closeStream(target, "downstream closed")
		return
	}
	var pending []byte
	finished := false
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
		for {
			index := bytes.Index(pending, []byte("\n\n"))
			if index < 0 {
				break
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
				streamError = sanitizeUpstream(msg.Failure)
				closeStream(target, streamError)
				return
			}
			if !emit(stream.Handle(msg)) {
				closeStream(target, "downstream closed")
				return
			}
			if msg.Finished {
				finished = true
			}
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
		return pluginabi.NewError("quota_exhausted", "Doubao rate limit or subscription quota reached: "+message, 429)
	case status >= 500:
		return pluginabi.NewError("upstream_error", "Doubao upstream failed: "+message, 502)
	case status == 0:
		return pluginabi.NewError("transport_error", "Doubao upstream transport failed", 503)
	default:
		return pluginabi.NewError("upstream_rejected", "Doubao rejected the request: "+message, status)
	}
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
