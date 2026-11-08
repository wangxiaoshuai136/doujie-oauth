package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func doubaoEvent(t *testing.T, eventType int, contentType int, content map[string]any) []byte {
	t.Helper()
	contentJSON, _ := json.Marshal(content)
	outer := map[string]any{
		"event_type": eventType,
		"event_data": map[string]any{
			"message": map[string]any{
				"content_type": contentType,
				"content":      string(contentJSON),
			},
		},
	}
	raw, _ := json.Marshal(outer)
	return raw
}

func TestParseDoubaoTextEvent(t *testing.T) {
	msg, ok := parseDoubaoEvent(doubaoEvent(t, 2001, 2001, map[string]any{"text": "你好"}))
	if !ok || msg.Text != "你好" || msg.ContentType != 2001 {
		t.Fatalf("unexpected message: %+v ok=%v", msg, ok)
	}
}

func TestParseDoubaoThinkingEvent(t *testing.T) {
	msg, ok := parseDoubaoEvent(doubaoEvent(t, 2001, 2008, map[string]any{"think": "推理中"}))
	if !ok || msg.Think != "推理中" {
		t.Fatalf("unexpected thinking message: %+v", msg)
	}
	msg, ok = parseDoubaoEvent(doubaoEvent(t, 2001, 10040, map[string]any{"title": "思考"}))
	if !ok || msg.Think != "思考" {
		t.Fatalf("unexpected think title: %+v", msg)
	}
}

func TestParseDoubaoFinishAndFailure(t *testing.T) {
	msg, ok := parseDoubaoEvent(doubaoEvent(t, 2003, 2003, map[string]any{"text": ""}))
	if !ok || !msg.Finished {
		t.Fatalf("expected finish event: %+v", msg)
	}
	raw, _ := json.Marshal(map[string]any{"event_type": 2005, "event_data": "blocked"})
	msg, ok = parseDoubaoEvent(raw)
	if !ok || !msg.Finished || msg.Failure != "blocked" {
		t.Fatalf("expected failure event: %+v", msg)
	}
}

func TestCollectDoubao(t *testing.T) {
	body := strings.Join([]string{
		"data: " + string(doubaoEvent(t, 2001, 2008, map[string]any{"think": "先想一想"})),
		"data: " + string(doubaoEvent(t, 2001, 2001, map[string]any{"text": "你好"})),
		"data: " + string(doubaoEvent(t, 2001, 2001, map[string]any{"text": "世界"})),
		"data: " + string(doubaoEvent(t, 2003, 2003, map[string]any{"text": ""})),
	}, "\n\n")
	text, think, err := collectDoubao([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if text != "你好世界" || think != "先想一想" {
		t.Fatalf("unexpected aggregate: %q %q", text, think)
	}
}

func TestFlattenConversation(t *testing.T) {
	body := anthropicRequest{
		System: json.RawMessage(`"你是助手"`),
		Messages: []anthropicMessage{
			{Role: "user", Content: json.RawMessage(`"问题一"`)},
			{Role: "assistant", Content: json.RawMessage(`"答案一"`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"问题二"}]`)},
		},
	}
	prompt, chars, err := flattenConversation(body)
	if err != nil {
		t.Fatal(err)
	}
	if chars == 0 || !strings.Contains(prompt, "你是助手") || !strings.Contains(prompt, "Assistant: 答案一") || !strings.Contains(prompt, "User: 问题二") {
		t.Fatalf("unexpected prompt: %s", prompt)
	}
}
