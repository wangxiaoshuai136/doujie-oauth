package main

import (
	"fmt"
	"net/http"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type httpSender interface {
	Do(pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error)
}

type cpaSender struct{ callbackID string }

func (s cpaSender) Do(req pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
	var out pluginapi.HTTPResponse
	err := hostRPC("host.http.do", map[string]any{"host_callback_id": s.callbackID, "request": req}, &out)
	if err != nil {
		return out, fmt.Errorf("upstream transport failed")
	}
	return out, nil
}

type hostStreamResponse struct {
	StatusCode int         `json:"status_code"`
	Headers    http.Header `json:"headers"`
	StreamID   string      `json:"stream_id"`
}

type hostReadResponse struct {
	Payload []byte `json:"payload"`
	Error   string `json:"error"`
	Done    bool   `json:"done"`
}

func openHostStream(sender httpSender, req pluginapi.HTTPRequest, callbackID string) (hostStreamResponse, error) {
	var out hostStreamResponse
	if s, ok := sender.(cpaSender); ok {
		callbackID = s.callbackID
	}
	if err := hostRPC("host.http.do_stream", map[string]any{"host_callback_id": callbackID, "request": req}, &out); err != nil {
		return out, fmt.Errorf("upstream stream failed")
	}
	return out, nil
}

func readHostStream(id string) (hostReadResponse, error) {
	var out hostReadResponse
	err := hostRPC("host.http.stream_read", map[string]any{"stream_id": id}, &out)
	if err != nil {
		return out, fmt.Errorf("upstream stream read failed")
	}
	if out.Error != "" {
		return out, fmt.Errorf("upstream stream failed")
	}
	return out, nil
}

func closeHostStream(id string) {
	if id != "" {
		_ = hostRPC("host.http.stream_close", map[string]any{"stream_id": id}, nil)
	}
}

func readWholeStream(id string) ([]byte, error) {
	defer closeHostStream(id)
	var out []byte
	for {
		chunk, err := readHostStream(id)
		if err != nil {
			return nil, err
		}
		out = append(out, chunk.Payload...)
		if len(out) > 8<<20 {
			return nil, fmt.Errorf("upstream response too large")
		}
		if chunk.Done {
			return out, nil
		}
	}
}

func emitStreamChunk(id string, payload []byte) error {
	return hostRPC("host.stream.emit", map[string]any{"stream_id": id, "payload": payload}, nil)
}

func closeStream(id, message string) {
	if id != "" {
		_ = hostRPC("host.stream.close", map[string]any{"stream_id": id, "error": message}, nil)
	}
}
