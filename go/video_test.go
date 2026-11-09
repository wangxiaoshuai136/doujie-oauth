package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func agentSSE(events ...map[string]any) []byte {
	var builder strings.Builder
	for _, event := range events {
		raw, _ := json.Marshal(event)
		builder.WriteString("event: STREAM_CHUNK\ndata: ")
		builder.Write(raw)
		builder.WriteString("\n\n")
	}
	return []byte(builder.String())
}

func videoCredentialJSON(t *testing.T) []byte {
	t.Helper()
	raw, err := json.Marshal(credential{
		Type:     credentialProvider,
		Cookies:  map[string]string{"sessionid": "secret-session", "passport_csrf_token": "csrf", "s_v_web_id": "verify_browser"},
		DeviceID: "7000000000000000",
		WebID:    "7600000000000000000",
		FP:       "verify_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func agentVideoEvent(taskID string, videoURL string, coverURL string) map[string]any {
	videoModel := map[string]any{
		"video_list": map[string]any{
			"video_1": map[string]any{
				"main_url": base64.StdEncoding.EncodeToString([]byte(videoURL)),
				"vwidth":   1280,
				"vheight":  720,
			},
		},
	}
	return map[string]any{
		"message_id": "assistant_1",
		"patch_op": []any{
			map[string]any{
				"patch_object": 1,
				"patch_type":   1,
				"patch_value": map[string]any{
					"content_block": []any{
						map[string]any{
							"block_type": 10084,
							"content": map[string]any{
								"video_model": videoModel,
								"poster_url":  coverURL,
							},
						},
					},
				},
			},
			map[string]any{
				"patch_object": 50,
				"patch_type":   1,
				"patch_value": map[string]any{
					"ext": map[string]any{
						"ai_creation_tool_list": `[{"task_id":"` + taskID + `","file_type":"video","tool_name":"text_to_video"}]`,
					},
				},
			},
		},
	}
}

func TestVideoModelSelection(t *testing.T) {
	for _, model := range []string{"doubao-video", "seedance-2.5", "seedance-2.0", "seedance-2.0-fast", "seedance-2.0-mini", "seedance-future-model"} {
		if !isVideoModel(model) {
			t.Fatalf("isVideoModel(%q) = false", model)
		}
	}
	if isVideoModel("doubao-seed-2.0") {
		t.Fatal("chat model was classified as video")
	}
	if got := videoModelLabel("seedance-2.0-mini"); got != "Seedance 2.0 Mini" {
		t.Fatalf("label = %q", got)
	}
}

func TestBuildVideoPayloadUsesAgentMode(t *testing.T) {
	payload := buildVideoPayload("a dog", "9:16", "5 秒", "seedance-2.0")
	clientMeta := mapValue(payload["client_meta"])
	if clientMeta["bot_id"] != videoAgentBotID {
		t.Fatalf("bot_id = %v", clientMeta["bot_id"])
	}
	message := mapValue(payload["messages"].([]any)[0])
	block := mapValue(message["content_block"].([]any)[0])
	text := fmtString(block, "content", "text_block", "text")
	if !strings.Contains(text, "Seedance 2.0") || !strings.Contains(text, "9:16") || !strings.Contains(text, "a dog") {
		t.Fatalf("agent prompt missing fields: %s", text)
	}
	option := mapValue(payload["option"])
	if asInt(option["agent_mode"]) != 1 || option["conversation_mode"] != 2 {
		t.Fatalf("unexpected agent option: %#v", option)
	}
}

func fmtString(root map[string]any, path ...string) string {
	current := any(root)
	for _, key := range path {
		current = mapValue(current)[key]
	}
	return strings.TrimSpace(strings.Trim(fmtSprint(current), "<nil>"))
}

func fmtSprint(value any) string {
	raw, _ := json.Marshal(value)
	return strings.Trim(string(raw), `"`)
}

func TestExtractVideoRequestPlanAndConfirmation(t *testing.T) {
	payload := []byte(`{"model":"doubao-video","messages":[{"role":"user","content":"a dog in the snow"}]}`)
	prompt, confirmed, err := extractVideoRequest("openai", payload)
	if err != nil || prompt != "a dog in the snow" || confirmed {
		t.Fatalf("plan request: %q %v %v", prompt, confirmed, err)
	}

	payload = []byte(`{"model":"doubao-video","messages":[{"role":"user","content":"a dog in the snow"},{"role":"assistant","content":"plan"},{"role":"user","content":"确认生成"}]}`)
	prompt, confirmed, err = extractVideoRequest("openai", payload)
	if err != nil || prompt != "a dog in the snow" || !confirmed {
		t.Fatalf("confirmed request: %q %v %v", prompt, confirmed, err)
	}

	payload = []byte(`{"model":"doubao-video","confirm":true,"messages":[{"role":"user","content":"a dog in the snow"}]}`)
	_, confirmed, err = extractVideoRequest("openai", payload)
	if err != nil || !confirmed {
		t.Fatalf("explicit confirm: %v %v", confirmed, err)
	}
}

func TestVideoRatioAndDuration(t *testing.T) {
	for payload, want := range map[string]string{
		`{"ratio":"1:1"}`:         "1:1",
		`{"aspect_ratio":"9:16"}`: "9:16",
		`{"size":"1280x720"}`:     "16:9",
		`{"size":"720x1280"}`:     "9:16",
		`{}`:                      "16:9",
	} {
		got, err := videoRatioFromPayload([]byte(payload))
		if err != nil || got != want {
			t.Fatalf("%s: got %q want %q err=%v", payload, got, want, err)
		}
	}
	if got := videoDurationFromPayload("make a 10 秒 clip", []byte(`{}`)); got != "10 秒" {
		t.Fatalf("duration = %q", got)
	}
}

func TestParseDoubaoAgentResponse(t *testing.T) {
	raw := agentSSE(agentVideoEvent("task_video", "https://cdn.example/video.mp4", "https://cdn.example/cover.jpg"))
	result, err := parseDoubaoAgentResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.TaskID != "task_video" || len(result.Videos) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if result.Videos[0].URL != "https://cdn.example/video.mp4" || result.Videos[0].CoverURL != "https://cdn.example/cover.jpg" {
		t.Fatalf("video = %#v", result.Videos[0])
	}
}

func TestExecuteVideoPlansBeforeGeneration(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	sender := &sequenceSender{}
	req := pluginapi.ExecutorRequest{
		Model:       "doubao-video",
		Format:      "openai",
		Payload:     []byte(`{"model":"doubao-video","messages":[{"role":"user","content":"a dog in the snow"}]}`),
		StorageJSON: videoCredentialJSON(t),
	}
	resp, err := executeVideo(req, sender)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resp.Payload), "视频生成方案") || !strings.Contains(string(resp.Payload), "确认生成") {
		t.Fatalf("plan response missing: %s", resp.Payload)
	}
	if len(sender.requests) != 0 {
		t.Fatalf("plan made upstream requests: %d", len(sender.requests))
	}
}

func TestExecuteVideoConfirmedOpenAIChat(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	sender := &sequenceSender{responses: []pluginapi.HTTPResponse{{
		StatusCode: http.StatusOK,
		Body:       agentSSE(agentVideoEvent("task_1", "https://cdn.example/final.mp4", "https://cdn.example/cover.jpg")),
	}}}
	req := pluginapi.ExecutorRequest{
		Model:       "seedance-2.0",
		Format:      "openai",
		Payload:     []byte(`{"model":"seedance-2.0","confirm":true,"messages":[{"role":"user","content":"a running dog"}]}`),
		StorageJSON: videoCredentialJSON(t),
	}
	resp, err := executeVideo(req, sender)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resp.Payload), "https://cdn.example/final.mp4") {
		t.Fatalf("payload lost video URL: %s", resp.Payload)
	}
	if len(sender.requests) != 1 || !strings.Contains(sender.requests[0].URL, "/chat/completion") {
		t.Fatalf("requests = %#v", sender.requests)
	}
	body := string(sender.requests[0].Body)
	if !strings.Contains(body, "Seedance 2.0") || !strings.Contains(body, "已确认执行") {
		t.Fatalf("submit body missing agent prompt: %s", body)
	}
	if !strings.Contains(sender.requests[0].URL, "aid=497858") || !strings.Contains(sender.requests[0].URL, "web_platform=browser") {
		t.Fatalf("agent URL missing updated params: %s", sender.requests[0].URL)
	}
}

func TestParseDoubaoAgentStreamError(t *testing.T) {
	raw := []byte("event: STREAM_ERROR\ndata: {\"error_code\":710022004,\"error_msg\":\"rate limited\"}\n\n")
	_, err := parseDoubaoAgentResponse(raw)
	if err == nil || !strings.Contains(err.Error(), "verification") {
		t.Fatalf("unexpected error: %v", err)
	}
}
