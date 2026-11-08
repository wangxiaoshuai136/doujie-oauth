package main

import (
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestFormatOf(t *testing.T) {
	cases := map[string]string{
		"claude":          "claude",
		"openai":          "openai",
		"openai-response": "openai-response",
		"responses":       "openai-response",
		"":                "claude",
		"gemini":          "claude",
	}
	for in, want := range cases {
		if got := formatOf(pluginapi.ExecutorRequest{Format: in}); got != want {
			t.Fatalf("formatOf(%q)=%q want %q", in, got, want)
		}
	}
	if got := formatOf(pluginapi.ExecutorRequest{SourceFormat: "openai"}); got != "openai" {
		t.Fatalf("source format fallback failed: %q", got)
	}
}

func TestExtractPromptOpenAI(t *testing.T) {
	payload := []byte(`{"model":"doubao","messages":[{"role":"system","content":"你是助手"},{"role":"user","content":"你好"},{"role":"assistant","content":"在的"},{"role":"user","content":[{"type":"text","text":"继续"}]}]}`)
	prompt, chars, deep, err := extractPrompt("openai", payload)
	if err != nil {
		t.Fatal(err)
	}
	if chars == 0 || deep || !strings.Contains(prompt, "你是助手") || !strings.Contains(prompt, "Assistant: 在的") || !strings.Contains(prompt, "User: 继续") {
		t.Fatalf("unexpected prompt: %s", prompt)
	}
}

func TestExtractPromptResponses(t *testing.T) {
	payload := []byte(`{"model":"doubao","instructions":"规则","input":[{"role":"user","content":[{"type":"input_text","text":"问题"}]}],"reasoning":{"effort":"high"}}`)
	prompt, chars, deep, err := extractPrompt("openai-response", payload)
	if err != nil {
		t.Fatal(err)
	}
	if chars == 0 || !deep || !strings.Contains(prompt, "规则") || !strings.Contains(prompt, "User: 问题") {
		t.Fatalf("unexpected prompt: %s deep=%v", prompt, deep)
	}
}

func TestOpenAIChatMessageShape(t *testing.T) {
	raw := string(openAIChatMessage("doubao", "答案", "思考"))
	for _, want := range []string{`"object":"chat.completion"`, `"content":"答案"`, `"reasoning_content":"思考"`, `"finish_reason":"stop"`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("missing %q in %s", want, raw)
		}
	}
}

func TestResponsesMessageShape(t *testing.T) {
	raw := string(responsesMessage("doubao", "答案", "思考"))
	for _, want := range []string{`"object":"response"`, `"type":"message"`, `"type":"output_text"`, `"type":"reasoning"`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("missing %q in %s", want, raw)
		}
	}
}

func TestOpenAIStreamFrames(t *testing.T) {
	stream := newOpenAIStream("doubao")
	var frames []string
	for _, frame := range stream.Start() {
		frames = append(frames, string(frame))
	}
	for _, frame := range stream.Handle(doubaoMessage{Text: "你好"}) {
		frames = append(frames, string(frame))
	}
	for _, frame := range stream.Finish() {
		frames = append(frames, string(frame))
	}
	joined := strings.Join(frames, "")
	for _, want := range []string{`"object":"chat.completion.chunk"`, `"content":"你好"`, `"finish_reason":"stop"`, "data: [DONE]"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %s", want, joined)
		}
	}
}

func TestResponsesStreamFrames(t *testing.T) {
	stream := newResponsesStream("doubao")
	var frames []string
	for _, frame := range stream.Handle(doubaoMessage{Text: "你好"}) {
		frames = append(frames, string(frame))
	}
	for _, frame := range stream.Finish() {
		frames = append(frames, string(frame))
	}
	joined := strings.Join(frames, "")
	for _, want := range []string{"event: response.created", "event: response.output_text.delta", "event: response.output_text.done", "event: response.completed"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %s", want, joined)
		}
	}
}
