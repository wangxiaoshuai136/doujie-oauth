package main

import (
	"encoding/json"
	"errors"
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

func TestParseChatVerificationChallenge(t *testing.T) {
	payload := []byte(`{"error_code":710022004,"error_msg":"rate limited","extra":{"decision":"{\"from\":\"shark_admin\",\"type\":\"verify\",\"subtype\":\"semantic_reasoning\",\"verify_scene\":\"doubao_message_web\"}"}}`)
	inThinking := false
	msg, ok := parseChatEvent(payload, &inThinking)
	if !ok || !msg.Finished || msg.Err == nil {
		t.Fatalf("expected structured verification error: %+v", msg)
	}
	if msg.Err.Code != 710022004 || msg.Err.Scene != "doubao_message_web" || msg.Err.Subtype != "semantic_reasoning" {
		t.Fatalf("unexpected verification details: %+v", msg.Err)
	}
	if !strings.Contains(msg.Failure, "710022004") {
		t.Fatalf("failure text lost the upstream code: %q", msg.Failure)
	}

	_, _, err := collectDoubaoChat([]byte("data: " + string(payload)))
	var streamErr *doubaoStreamError
	if !errors.As(err, &streamErr) || streamErr.Code != 710022004 {
		t.Fatalf("collectDoubaoChat did not preserve the verification error: %#v", err)
	}
}

func TestParseDoubaoStructuredFailureEvent(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"event_type": 2005,
		"event_data": map[string]any{
			"error_code": 710022004,
			"error_msg":  "rate limited",
			"extra": map[string]any{
				"decision": `{"type":"verify","subtype":"semantic_reasoning","verify_scene":"doubao_message_web"}`,
			},
		},
	})
	msg, ok := parseDoubaoEvent(raw)
	if !ok || !msg.Finished || msg.Err == nil {
		t.Fatalf("expected structured samantha failure: %+v", msg)
	}
	if msg.Err.Code != 710022004 || msg.Err.Scene != "doubao_message_web" || msg.Err.Subtype != "semantic_reasoning" {
		t.Fatalf("unexpected samantha verification details: %+v", msg.Err)
	}
}

func TestParseChatEventSeparatesThinkingFromAnswer(t *testing.T) {
	inThinking := false
	thinking := []byte(`{"content":{"content_block":[{"block_type":10040,"content":{"thinking_block":{"finish_title":"思考"}}}]},"content_type":9999}`)
	if _, ok := parseChatEvent(thinking, &inThinking); !ok || !inThinking {
		t.Fatalf("thinking block did not enable thinking: ok=%v inThinking=%v", ok, inThinking)
	}
	answer := []byte(`{"message_id":"m","patch_op":[{"patch_value":{"content_block":[{"block_type":10000,"content":{"text_block":{"text":"1"}}}]}}]}`)
	msg, ok := parseChatEvent(answer, &inThinking)
	if !ok || msg.Text != "1" || msg.Think != "" || inThinking {
		t.Fatalf("unexpected answer: %+v ok=%v inThinking=%v", msg, ok, inThinking)
	}
}

func TestParseChatEventStreamMessageNotify(t *testing.T) {
	inThinking := false
	raw := []byte(`{"content":{"content_type":1,"content":"{\"text\":\"收到\",\"text_tags\":[]}","model_content":"收到"},"meta":{"message_id":"m"}}`)
	msg, ok := parseChatEvent(raw, &inThinking)
	if !ok || msg.Text != "收到" || msg.Think != "" {
		t.Fatalf("unexpected stream message: %+v ok=%v", msg, ok)
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
	prompt, chars, err := flattenConversation(body, 0)
	if err != nil {
		t.Fatal(err)
	}
	if chars == 0 || !strings.Contains(prompt, "你是助手") || !strings.Contains(prompt, "Assistant: 答案一") || !strings.Contains(prompt, "User: 问题二") {
		t.Fatalf("unexpected prompt: %s", prompt)
	}
}
