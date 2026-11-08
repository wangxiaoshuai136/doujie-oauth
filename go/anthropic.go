package main

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

type anthropicStream struct {
	model    string
	id       string
	dataOnly bool
	started  bool
	index    int
	open     string
	chars    int
}

func newAnthropicStream(model string, dataOnly bool) *anthropicStream {
	return &anthropicStream{model: model, id: newMessageID(), dataOnly: dataOnly}
}

func (s *anthropicStream) frame(event string, payload map[string]any) []byte {
	payload["type"] = event
	raw, _ := json.Marshal(payload)
	if s.dataOnly {
		return append(append([]byte("data: "), raw...), '\n', '\n')
	}
	out := append([]byte("event: "+event+"\n"), []byte("data: ")...)
	out = append(out, raw...)
	return append(out, '\n', '\n')
}

func (s *anthropicStream) Start() [][]byte {
	if s.started {
		return nil
	}
	s.started = true
	return [][]byte{s.frame("message_start", map[string]any{
		"message": map[string]any{
			"id":            s.id,
			"type":          "message",
			"role":          "assistant",
			"model":         s.model,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": 0, "output_tokens": 0},
		},
	})}
}

func (s *anthropicStream) switchBlock(kind string) [][]byte {
	if s.open == kind {
		return nil
	}
	var frames [][]byte
	if s.open != "" {
		frames = append(frames, s.frame("content_block_stop", map[string]any{"index": s.index}))
		s.index++
	}
	s.open = kind
	block := map[string]any{"type": kind}
	if kind == "thinking" {
		block["thinking"] = ""
	} else {
		block["text"] = ""
	}
	return append(frames, s.frame("content_block_start", map[string]any{"index": s.index, "content_block": block}))
}

func (s *anthropicStream) Handle(msg doubaoMessage) [][]byte {
	var frames [][]byte
	if strings.TrimSpace(msg.Think) != "" {
		frames = append(frames, s.Start()...)
		frames = append(frames, s.switchBlock("thinking")...)
		s.chars += utf8.RuneCountInString(msg.Think)
		frames = append(frames, s.frame("content_block_delta", map[string]any{
			"index": s.index,
			"delta": map[string]any{"type": "thinking_delta", "thinking": msg.Think},
		}))
	}
	if msg.Text != "" {
		frames = append(frames, s.Start()...)
		frames = append(frames, s.switchBlock("text")...)
		s.chars += utf8.RuneCountInString(msg.Text)
		frames = append(frames, s.frame("content_block_delta", map[string]any{
			"index": s.index,
			"delta": map[string]any{"type": "text_delta", "text": msg.Text},
		}))
	}
	return frames
}

func (s *anthropicStream) Finish() [][]byte {
	frames := s.Start()
	if s.open != "" {
		frames = append(frames, s.frame("content_block_stop", map[string]any{"index": s.index}))
		s.open = ""
	}
	frames = append(frames, s.frame("message_delta", map[string]any{
		"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": map[string]any{"output_tokens": estimateTokens(s.chars)},
	}))
	frames = append(frames, s.frame("message_stop", map[string]any{}))
	return frames
}

func estimateTokens(chars int) int {
	if chars <= 0 {
		return 1
	}
	return chars/2 + 1
}

func buildAnthropicMessage(model, text, think string) []byte {
	content := make([]any, 0, 2)
	if strings.TrimSpace(think) != "" {
		content = append(content, map[string]any{"type": "thinking", "thinking": think})
	}
	content = append(content, map[string]any{"type": "text", "text": text})
	chars := utf8.RuneCountInString(text) + utf8.RuneCountInString(think)
	msg := map[string]any{
		"id":            newMessageID(),
		"type":          "message",
		"role":          "assistant",
		"model":         model,
		"content":       content,
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage":         map[string]any{"input_tokens": 0, "output_tokens": estimateTokens(chars)},
	}
	raw, _ := json.Marshal(msg)
	return raw
}
