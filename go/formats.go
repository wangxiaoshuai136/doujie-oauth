package main

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type promptTurn struct {
	Role string
	Text string
}

func formatOf(req pluginapi.ExecutorRequest) string {
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "" {
		format = strings.ToLower(strings.TrimSpace(req.SourceFormat))
	}
	switch format {
	case "openai":
		return "openai"
	case "openai-response", "responses":
		return "openai-response"
	default:
		return "claude"
	}
}

func modelFromPayload(payload []byte) string {
	var envelope struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(payload, &envelope) == nil {
		return strings.TrimSpace(envelope.Model)
	}
	return ""
}

func buildPrompt(system string, turns []promptTurn) (string, int, error) {
	var parts []string
	if strings.TrimSpace(system) != "" {
		parts = append(parts, "System instructions:\n"+system)
	}
	var rendered []string
	for _, turn := range turns {
		if strings.TrimSpace(turn.Text) == "" {
			continue
		}
		label := "User"
		if strings.EqualFold(turn.Role, "assistant") {
			label = "Assistant"
		}
		rendered = append(rendered, label+": "+turn.Text)
	}
	if len(rendered) == 0 {
		return "", 0, errors.New("messages must contain text")
	}
	parts = append(parts, "Conversation:\n"+strings.Join(rendered, "\n\n"))
	parts = append(parts, "Reply as the assistant to the final user message.")
	prompt := strings.Join(parts, "\n\n")
	chars := utf8.RuneCountInString(prompt)
	if chars > maxPromptChars {
		return "", 0, errors.New("prompt exceeds the Doubao web context limit")
	}
	return prompt, chars, nil
}

type openAIChatRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
	Stream          bool   `json:"stream"`
	ReasoningEffort string `json:"reasoning_effort"`
}

type responsesRequest struct {
	Model        string          `json:"model"`
	Instructions json.RawMessage `json:"instructions"`
	Input        json.RawMessage `json:"input"`
	Stream       bool            `json:"stream"`
	Reasoning    *struct {
		Effort string `json:"effort"`
	} `json:"reasoning"`
}

func extractPrompt(format string, payload []byte) (string, int, bool, error) {
	switch format {
	case "openai":
		var body openAIChatRequest
		if json.Unmarshal(payload, &body) != nil {
			return "", 0, false, errors.New("invalid OpenAI Chat Completions request")
		}
		var system []string
		var turns []promptTurn
		for _, message := range body.Messages {
			text := flattenBlocks(message.Content)
			if strings.EqualFold(message.Role, "system") || strings.EqualFold(message.Role, "developer") {
				if strings.TrimSpace(text) != "" {
					system = append(system, text)
				}
				continue
			}
			turns = append(turns, promptTurn{Role: message.Role, Text: text})
		}
		prompt, chars, err := buildPrompt(strings.Join(system, "\n\n"), turns)
		deep := strings.EqualFold(body.ReasoningEffort, "high") || strings.EqualFold(body.ReasoningEffort, "medium")
		return prompt, chars, deep, err
	case "openai-response":
		var body responsesRequest
		if json.Unmarshal(payload, &body) != nil {
			return "", 0, false, errors.New("invalid OpenAI Responses request")
		}
		system := flattenBlocks(body.Instructions)
		var turns []promptTurn
		if text, ok := rawText(body.Input); ok {
			turns = append(turns, promptTurn{Role: "user", Text: text})
		} else {
			var items []struct {
				Type    string          `json:"type"`
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			}
			if json.Unmarshal(body.Input, &items) == nil {
				for _, item := range items {
					if item.Type != "" && item.Type != "message" {
						continue
					}
					turns = append(turns, promptTurn{Role: item.Role, Text: flattenBlocks(item.Content)})
				}
			}
		}
		prompt, chars, err := buildPrompt(system, turns)
		deep := body.Reasoning != nil && (strings.EqualFold(body.Reasoning.Effort, "high") || strings.EqualFold(body.Reasoning.Effort, "medium"))
		return prompt, chars, deep, err
	default:
		var body anthropicRequest
		if json.Unmarshal(payload, &body) != nil {
			return "", 0, false, errors.New("invalid Anthropic Messages request")
		}
		system := flattenBlocks(body.System)
		var turns []promptTurn
		for _, message := range body.Messages {
			turns = append(turns, promptTurn{Role: message.Role, Text: flattenBlocks(message.Content)})
		}
		prompt, chars, err := buildPrompt(system, turns)
		deep := body.Thinking != nil && strings.EqualFold(body.Thinking.Type, "enabled")
		return prompt, chars, deep, err
	}
}

func rawText(raw json.RawMessage) (string, bool) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, true
	}
	return "", false
}

type streamEncoder interface {
	Start() [][]byte
	Handle(doubaoMessage) [][]byte
	Finish() [][]byte
}

func newStreamEncoder(format, model string, dataOnly bool) streamEncoder {
	switch format {
	case "openai":
		return newOpenAIStream(model)
	case "openai-response":
		return newResponsesStream(model)
	default:
		return newAnthropicStream(model, dataOnly)
	}
}

func encodeMessage(format, model, text, think string) []byte {
	switch format {
	case "openai":
		return openAIChatMessage(model, text, think)
	case "openai-response":
		return responsesMessage(model, text, think)
	default:
		return buildAnthropicMessage(model, text, think)
	}
}

func nowUnix() int64 { return time.Now().Unix() }

func openAIChatMessage(model, text, think string) []byte {
	message := map[string]any{"role": "assistant", "content": text}
	if think != "" {
		message["reasoning_content"] = think
	}
	out := map[string]any{
		"id":      "chatcmpl-" + strings.ReplaceAll(randomUUID(), "-", "")[:24],
		"object":  "chat.completion",
		"created": nowUnix(),
		"model":   model,
		"choices": []any{map[string]any{
			"index":         0,
			"message":       message,
			"finish_reason": "stop",
		}},
		"usage": map[string]any{
			"prompt_tokens":     0,
			"completion_tokens": estimateTokens(utf8.RuneCountInString(text) + utf8.RuneCountInString(think)),
			"total_tokens":      estimateTokens(utf8.RuneCountInString(text) + utf8.RuneCountInString(think)),
		},
	}
	raw, _ := json.Marshal(out)
	return raw
}

func responsesMessage(model, text, think string) []byte {
	tokens := estimateTokens(utf8.RuneCountInString(text) + utf8.RuneCountInString(think))
	content := []any{map[string]any{"type": "output_text", "text": text, "annotations": []any{}}}
	output := []any{map[string]any{
		"id":      "msg_" + strings.ReplaceAll(randomUUID(), "-", "")[:24],
		"type":    "message",
		"status":  "completed",
		"role":    "assistant",
		"content": content,
	}}
	if think != "" {
		output = append([]any{map[string]any{
			"id":      "rs_" + strings.ReplaceAll(randomUUID(), "-", "")[:24],
			"type":    "reasoning",
			"summary": []any{map[string]any{"type": "summary_text", "text": think}},
		}}, output...)
	}
	out := map[string]any{
		"id":         "resp_" + strings.ReplaceAll(randomUUID(), "-", "")[:24],
		"object":     "response",
		"created_at": nowUnix(),
		"status":     "completed",
		"model":      model,
		"output":     output,
		"usage":      map[string]any{"input_tokens": 0, "output_tokens": tokens, "total_tokens": tokens},
	}
	raw, _ := json.Marshal(out)
	return raw
}

func jsonFrame(event string, payload map[string]any) []byte {
	if event != "" {
		payload["type"] = event
	}
	raw, _ := json.Marshal(payload)
	if event == "" {
		return append(append([]byte("data: "), raw...), '\n', '\n')
	}
	out := append([]byte("event: "+event+"\n"), []byte("data: ")...)
	out = append(out, raw...)
	return append(out, '\n', '\n')
}

func dataFrame(payload map[string]any) []byte {
	raw, _ := json.Marshal(payload)
	return append(append([]byte("data: "), raw...), '\n', '\n')
}

type openAIStream struct {
	id       string
	model    string
	started  bool
	finished bool
	chars    int
	index    int
}

func newOpenAIStream(model string) *openAIStream {
	return &openAIStream{id: "chatcmpl-" + strings.ReplaceAll(randomUUID(), "-", "")[:24], model: model}
}

func (s *openAIStream) chunk(delta map[string]any, finish any) []byte {
	return dataFrame(map[string]any{
		"id":      s.id,
		"object":  "chat.completion.chunk",
		"created": nowUnix(),
		"model":   s.model,
		"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
	})
}

func (s *openAIStream) Start() [][]byte {
	if s.started {
		return nil
	}
	s.started = true
	return [][]byte{s.chunk(map[string]any{"role": "assistant", "content": ""}, nil)}
}

func (s *openAIStream) Handle(msg doubaoMessage) [][]byte {
	var frames [][]byte
	frames = append(frames, s.Start()...)
	if msg.Think != "" {
		s.chars += utf8.RuneCountInString(msg.Think)
		frames = append(frames, s.chunk(map[string]any{"reasoning_content": msg.Think}, nil))
	}
	if msg.Text != "" {
		s.chars += utf8.RuneCountInString(msg.Text)
		frames = append(frames, s.chunk(map[string]any{"content": msg.Text}, nil))
	}
	return frames
}

func (s *openAIStream) Finish() [][]byte {
	if s.finished {
		return nil
	}
	s.finished = true
	frames := s.Start()
	frames = append(frames, s.chunk(map[string]any{}, "stop"))
	frames = append(frames, []byte("data: [DONE]\n\n"))
	return frames
}

type responsesStream struct {
	id          string
	messageID   string
	reasoningID string
	model       string
	started     bool
	finished    bool
	text        strings.Builder
	think       strings.Builder
}

func newResponsesStream(model string) *responsesStream {
	return &responsesStream{
		id:          "resp_" + strings.ReplaceAll(randomUUID(), "-", "")[:24],
		messageID:   "msg_" + strings.ReplaceAll(randomUUID(), "-", "")[:24],
		reasoningID: "rs_" + strings.ReplaceAll(randomUUID(), "-", "")[:24],
		model:       model,
	}
}

func (s *responsesStream) Start() [][]byte {
	if s.started {
		return nil
	}
	s.started = true
	return [][]byte{jsonFrame("response.created", map[string]any{
		"response": map[string]any{
			"id": s.id, "object": "response", "created_at": nowUnix(),
			"status": "in_progress", "model": s.model, "output": []any{},
		},
	})}
}

func (s *responsesStream) Handle(msg doubaoMessage) [][]byte {
	var frames [][]byte
	frames = append(frames, s.Start()...)
	if msg.Think != "" {
		s.think.WriteString(msg.Think)
		frames = append(frames, jsonFrame("response.reasoning_summary_text.delta", map[string]any{
			"item_id": s.reasoningID, "output_index": 0, "summary_index": 0, "delta": msg.Think,
		}))
	}
	if msg.Text != "" {
		if s.text.Len() == 0 {
			frames = append(frames, jsonFrame("response.output_item.added", map[string]any{
				"output_index": 0,
				"item":         map[string]any{"id": s.messageID, "type": "message", "status": "in_progress", "role": "assistant", "content": []any{}},
			}))
			frames = append(frames, jsonFrame("response.content_part.added", map[string]any{
				"item_id": s.messageID, "output_index": 0, "content_index": 0,
				"part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}},
			}))
		}
		s.text.WriteString(msg.Text)
		frames = append(frames, jsonFrame("response.output_text.delta", map[string]any{
			"item_id": s.messageID, "output_index": 0, "content_index": 0, "delta": msg.Text,
		}))
	}
	return frames
}

func (s *responsesStream) Finish() [][]byte {
	if s.finished {
		return nil
	}
	s.finished = true
	frames := s.Start()
	text := s.text.String()
	think := s.think.String()
	if text == "" {
		return append(frames, jsonFrame("response.completed", map[string]any{"response": s.finalResponse(text, think)}))
	}
	frames = append(frames, jsonFrame("response.output_text.done", map[string]any{
		"item_id": s.messageID, "output_index": 0, "content_index": 0, "text": text,
	}))
	frames = append(frames, jsonFrame("response.content_part.done", map[string]any{
		"item_id": s.messageID, "output_index": 0, "content_index": 0,
		"part": map[string]any{"type": "output_text", "text": text, "annotations": []any{}},
	}))
	frames = append(frames, jsonFrame("response.output_item.done", map[string]any{
		"output_index": 0,
		"item": map[string]any{"id": s.messageID, "type": "message", "status": "completed", "role": "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": text, "annotations": []any{}}}},
	}))
	return append(frames, jsonFrame("response.completed", map[string]any{"response": s.finalResponse(text, think)}))
}

func (s *responsesStream) finalResponse(text, think string) map[string]any {
	tokens := estimateTokens(utf8.RuneCountInString(text) + utf8.RuneCountInString(think))
	output := []any{map[string]any{
		"id": s.messageID, "type": "message", "status": "completed", "role": "assistant",
		"content": []any{map[string]any{"type": "output_text", "text": text, "annotations": []any{}}},
	}}
	if think != "" {
		output = append([]any{map[string]any{
			"id": s.reasoningID, "type": "reasoning",
			"summary": []any{map[string]any{"type": "summary_text", "text": think}},
		}}, output...)
	}
	return map[string]any{
		"id": s.id, "object": "response", "created_at": nowUnix(), "status": "completed",
		"model": s.model, "output": output,
		"usage": map[string]any{"input_tokens": 0, "output_tokens": tokens, "total_tokens": tokens},
	}
}

func formatLabel(format string) string {
	switch format {
	case "openai":
		return "OpenAI Chat Completions"
	case "openai-response":
		return "OpenAI Responses"
	default:
		return "Anthropic Messages"
	}
}
