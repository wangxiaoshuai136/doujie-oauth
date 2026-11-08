package main

import (
	"strings"
	"testing"
)

func TestAnthropicStreamEventFrames(t *testing.T) {
	stream := newAnthropicStream("doubao", false)
	var frames []string
	for _, frame := range stream.Start() {
		frames = append(frames, string(frame))
	}
	for _, frame := range stream.Handle(doubaoMessage{Think: "想一想", Text: "答案"}) {
		frames = append(frames, string(frame))
	}
	for _, frame := range stream.Finish() {
		frames = append(frames, string(frame))
	}
	joined := strings.Join(frames, "")
	for _, want := range []string{"event: message_start", "thinking_delta", "event: content_block_start", "text_delta", "event: message_stop", "content_block_stop"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in stream:\n%s", want, joined)
		}
	}
	if strings.Count(joined, "event: content_block_start") != 2 {
		t.Fatalf("expected a thinking and a text block:\n%s", joined)
	}
}

func TestAnthropicStreamDataOnly(t *testing.T) {
	stream := newAnthropicStream("doubao", true)
	frames := stream.Start()
	if len(frames) != 1 || !strings.HasPrefix(string(frames[0]), "data: ") || strings.Contains(string(frames[0]), "event:") {
		t.Fatalf("unexpected data-only frame: %s", frames)
	}
	stop := stream.Finish()
	if len(stop) == 0 || !strings.Contains(strings.Join(frameStrings(stop), ""), "message_stop") {
		t.Fatalf("missing message_stop: %s", frameStrings(stop))
	}
}

func frameStrings(frames [][]byte) []string {
	out := make([]string, 0, len(frames))
	for _, frame := range frames {
		out = append(out, string(frame))
	}
	return out
}

func TestBuildAnthropicMessage(t *testing.T) {
	raw := buildAnthropicMessage("doubao", "答案", "")
	text := string(raw)
	if !strings.Contains(text, `"type":"message"`) || !strings.Contains(text, "答案") || !strings.Contains(text, `"stop_reason":"end_turn"`) {
		t.Fatalf("unexpected message: %s", text)
	}
}
