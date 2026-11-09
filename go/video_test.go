package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func videoSSE(events ...map[string]any) []byte {
	var builder strings.Builder
	for _, event := range events {
		raw, _ := json.Marshal(event)
		builder.WriteString("data: ")
		builder.Write(raw)
		builder.WriteString("\n\n")
	}
	return []byte(builder.String())
}

func videoTaskEvent(taskID string) map[string]any {
	return map[string]any{
		"event_type": 2001,
		"event_data": map[string]any{
			"fin_reason": map[string]any{
				"reason":     1,
				"async_task": map[string]any{"id": taskID},
			},
		},
	}
}

func videoOutputEvent(content map[string]any) map[string]any {
	raw, _ := json.Marshal(content)
	return map[string]any{
		"event_type": 2001,
		"event_data": map[string]any{
			"message": map[string]any{
				"content_type": 2021,
				"content":      string(raw),
			},
		},
	}
}

func videoCredentialJSON(t *testing.T) []byte {
	t.Helper()
	raw, err := json.Marshal(credential{
		Type:     credentialProvider,
		Cookies:  map[string]string{"sessionid": "secret-session", "passport_csrf_token": "csrf"},
		DeviceID: "7000000000000000",
		WebID:    "7600000000000000000",
		FP:       "verify_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestVideoModelAliases(t *testing.T) {
	for _, model := range []string{"doubao-video", "seedance-2.5", "doubao-seedance-2.5", "seedance_2.5"} {
		if !isVideoModel(model) {
			t.Fatalf("isVideoModel(%q) = false", model)
		}
	}
	if isVideoModel("doubao-seed-2.0") {
		t.Fatal("chat model was classified as video")
	}
}

func TestVideoPayloadFixesSeedance25(t *testing.T) {
	payload := videoPayload("a dog", "9:16", videoModelVersion)
	message := payload["messages"].([]any)[0].(map[string]any)
	if message["content_type"] != videoContentType {
		t.Fatalf("content_type = %v", message["content_type"])
	}
	content := mapValue(message["content"])
	if content["model_version"] != videoModelVersion || content["ratio"] != "9:16" || content["text"] != "a dog" {
		t.Fatalf("unexpected content: %#v", content)
	}
	skill := mapValue(message["skill"])
	if asInt(skill["skill_type"]) != videoSkillType {
		t.Fatalf("skill_type = %v", skill["skill_type"])
	}
}

func TestExtractVideoPromptUsesLastUserMessage(t *testing.T) {
	payload := []byte(`{"model":"doubao-video","messages":[{"role":"system","content":"system"},{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":[{"type":"text","text":"last prompt"}]}]}`)
	prompt, err := extractVideoPrompt("openai", payload)
	if err != nil {
		t.Fatal(err)
	}
	if prompt != "last prompt" {
		t.Fatalf("prompt = %q", prompt)
	}
}

func TestVideoRatioFromPayload(t *testing.T) {
	tests := map[string]string{
		`{"ratio":"1:1"}`:         "1:1",
		`{"aspect_ratio":"9:16"}`: "9:16",
		`{"size":"1280x720"}`:     "16:9",
		`{"size":"720x1280"}`:     "9:16",
		`{"size":"1024x1024"}`:    "1:1",
		`{}`:                      "16:9",
	}
	for payload, want := range tests {
		got, err := videoRatioFromPayload([]byte(payload))
		if err != nil {
			t.Fatalf("%s: %v", payload, err)
		}
		if got != want {
			t.Fatalf("%s: got %q want %q", payload, got, want)
		}
	}
	if _, err := videoRatioFromPayload([]byte(`{"ratio":"4:3"}`)); err == nil {
		t.Fatal("expected an invalid ratio error")
	}
}

func TestParseDoubaoVideoResponse(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("https://cdn.example/encoded.mp4"))
	raw := videoSSE(
		videoTaskEvent("task_video"),
		videoOutputEvent(map[string]any{
			"data": []any{
				map[string]any{"video_url": "https://cdn.example/direct.mp4", "duration": 5.0},
				map[string]any{"video_model": map[string]any{"video_list": map[string]any{"q": map[string]any{"main_url": encoded}}}},
			},
		}),
	)
	taskID, videos, _, err := parseDoubaoVideoResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if taskID != "task_video" || len(videos) != 2 {
		t.Fatalf("task=%q videos=%#v", taskID, videos)
	}
	if videos[0].URL != "https://cdn.example/direct.mp4" || videos[1].URL != "https://cdn.example/encoded.mp4" {
		t.Fatalf("unexpected URLs: %#v", videos)
	}
}

func TestExecuteVideoOpenAIChat(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	sender := &sequenceSender{responses: []pluginapi.HTTPResponse{
		{StatusCode: http.StatusOK, Body: videoSSE(videoTaskEvent("task_1"))},
		{StatusCode: http.StatusOK, Body: videoSSE(videoOutputEvent(map[string]any{
			"data": []any{map[string]any{"video_url": "https://cdn.example/final.mp4", "duration": 5.0}},
		}))},
	}}
	req := pluginapi.ExecutorRequest{
		Model:       "doubao-video",
		Format:      "openai",
		Payload:     []byte(`{"model":"doubao-video","messages":[{"role":"user","content":"a running dog"}]}`),
		StorageJSON: videoCredentialJSON(t),
	}
	resp, err := executeVideo(req, sender)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resp.Payload), "https://cdn.example/final.mp4") {
		t.Fatalf("payload lost video URL: %s", resp.Payload)
	}
	if len(sender.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(sender.requests))
	}
	if !strings.Contains(sender.requests[0].URL, "/samantha/chat/completion") {
		t.Fatalf("submit URL = %s", sender.requests[0].URL)
	}
	if !strings.Contains(sender.requests[1].URL, "/samantha/chat/async/stream") {
		t.Fatalf("poll URL = %s", sender.requests[1].URL)
	}
	if strings.Contains(sender.requests[1].URL, "fp=") {
		t.Fatalf("async URL contains fp: %s", sender.requests[1].URL)
	}
	body := string(sender.requests[0].Body)
	if !strings.Contains(body, "seedance_2.5") || !strings.Contains(body, "2020") || !strings.Contains(body, "17") {
		t.Fatalf("submit body missing video protocol fields: %s", body)
	}
	if !strings.Contains(body, "a running dog") {
		t.Fatalf("submit body missing prompt: %s", body)
	}
}

func TestParseDoubaoVideoGatewayError(t *testing.T) {
	raw := []byte("event: gateway-error\ndata: {\"code\":710012001,\"message\":\"expired\"}\n\n")
	_, _, _, err := parseDoubaoVideoResponse(raw)
	if err == nil || !strings.Contains(err.Error(), "sign in again") {
		t.Fatalf("unexpected error: %v", err)
	}
}
