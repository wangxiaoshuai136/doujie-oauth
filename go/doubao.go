package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const maxPromptChars = 120000

type anthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	System    json.RawMessage    `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
	Stream    bool               `json:"stream"`
	MaxTokens int                `json:"max_tokens"`
	Thinking  *struct {
		Type         string `json:"type"`
		BudgetTokens int    `json:"budget_tokens"`
	} `json:"thinking"`
}

type preparedRequest struct {
	HTTP       pluginapi.HTTPRequest
	Model      string
	AuthID     string
	DeepThink  bool
	InputChars int
	Endpoint   string
	Format     string
}

func buildRequest(req pluginapi.ExecutorRequest, sender httpSender) (preparedRequest, error) {
	var out preparedRequest
	c, err := readCredential(req.StorageJSON)
	if err != nil {
		return out, pluginabi.NewError("invalid_auth", "invalid Doubao credential", 401)
	}
	if len(req.Payload) == 0 {
		return out, pluginabi.NewError("invalid_request", "empty request payload", 400)
	}
	format := formatOf(req)
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = modelFromPayload(req.Payload)
	}
	if model == "" {
		model = "doubao"
	}
	prompt, chars, requestDeep, err := extractPrompt(format, req.Payload)
	if err != nil {
		return out, pluginabi.NewError("invalid_request", err.Error(), 400)
	}
	cfg := getConfig()
	profile := c.profile()
	deep := cfg.DeepThink || wantsDeepThink(model) || requestDeep
	params := securityParams(c, model)
	endpoint := cfg.Endpoint
	if endpoint != "chat" {
		endpoint = "samantha"
	}
	var path string
	var sent map[string]any
	if endpoint == "chat" {
		path = "/chat/completion?"
		sent = chatCompletionPayload(prompt, deep, c.FP, c.botID())
	} else {
		path = "/samantha/chat/completion?"
		sent = samanthaPayload(prompt, deep, c.botID())
	}
	target := profile.Root + path + params.Encode()
	raw, err := json.Marshal(sent)
	if err != nil {
		return out, pluginabi.NewError("serialization_error", "cannot encode Doubao request", 500)
	}
	headers := http.Header{
		"Accept":          {"text/event-stream"},
		"Content-Type":    {"application/json"},
		"Agw-Js-Conv":     {"str"},
		"User-Agent":      {chromeUA},
		"Referer":         {profile.Root + "/chat/"},
		"Origin":          {profile.Root},
		"Accept-Language": {profile.Language + ",en;q=0.8"},
		"Cookie":          {c.cookieHeader()},
	}
	if endpoint == "chat" {
		csrf := strings.TrimSpace(c.Cookies["passport_csrf_token"])
		if csrf == "" {
			csrf = strings.TrimSpace(c.Cookies["passport_csrf_token_default"])
		}
		if csrf != "" {
			headers.Set("X-Tt-Passport-Csrf-Token", csrf)
		}
	}
	httpReq := pluginapi.HTTPRequest{Method: http.MethodPost, URL: target, Headers: headers, Body: raw}
	if signer := newSigner(cfg); signer != nil {
		httpReq, err = signer.Sign(httpReq, sender)
		if err != nil {
			return out, pluginabi.NewError("signer_error", err.Error(), 503)
		}
	}
	return preparedRequest{HTTP: httpReq, Model: model, AuthID: req.AuthID, DeepThink: deep, InputChars: chars, Endpoint: endpoint, Format: format}, nil
}

func securityParams(c credential, model string) url.Values {
	profile := c.profile()
	params := url.Values{}
	params.Set("aid", profile.ChatAID)
	params.Set("real_aid", profile.ChatAID)
	params.Set("device_id", c.DeviceID)
	params.Set("tea_uuid", c.DeviceID)
	params.Set("web_id", c.WebID)
	params.Set("device_platform", "web")
	params.Set("language", profile.Language)
	params.Set("region", profile.Region)
	params.Set("sys_region", profile.SysRegion)
	params.Set("pkg_type", "release_version")
	params.Set("version_code", "20800")
	pcVersion := profile.WebVersion
	if profile.ID == siteCN {
		pcVersion = getConfig().AppVersion
	}
	params.Set("pc_version", pcVersion)
	if profile.ID == siteCN {
		params.Set("chromium_version", chromiumVer)
		params.Set("client_platform", "pc_client")
		params.Set("runtime", "web")
		params.Set("runtime_version", "3.5.4")
	}
	params.Set("samantha_web", "1")
	params.Set("use-olympus-account", "1")
	params.Set("fp", c.FP)
	params.Set("web_tab_id", randomUUID())
	if token := strings.TrimSpace(c.Cookies["msToken"]); token != "" {
		params.Set("msToken", token)
	}
	return params
}

func flattenConversation(body anthropicRequest) (string, int, error) {
	var parts []string
	if text := flattenBlocks(body.System); strings.TrimSpace(text) != "" {
		parts = append(parts, "System instructions:\n"+text)
	}
	var turns []string
	for _, message := range body.Messages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		label := "User"
		if role == "assistant" {
			label = "Assistant"
		}
		text := flattenBlocks(message.Content)
		if strings.TrimSpace(text) == "" {
			continue
		}
		turns = append(turns, label+": "+text)
	}
	if len(turns) == 0 {
		return "", 0, errors.New("messages must contain text")
	}
	parts = append(parts, "Conversation:\n"+strings.Join(turns, "\n\n"))
	parts = append(parts, "Reply as the assistant to the final user message.")
	prompt := strings.Join(parts, "\n\n")
	if utf8.RuneCountInString(prompt) > maxPromptChars {
		return "", 0, errors.New("prompt exceeds the Doubao web context limit")
	}
	return prompt, utf8.RuneCountInString(prompt), nil
}

func flattenBlocks(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []map[string]any
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var out []string
	for _, block := range blocks {
		switch block["type"] {
		case "text", "input_text", "output_text":
			if text, ok := block["text"].(string); ok {
				out = append(out, text)
			}
		case "tool_result", "tool_use":
			out = append(out, "[tool output omitted: the Doubao web executor does not run tools]")
		case "image":
			out = append(out, "[image omitted: the Doubao web executor is text-only]")
		case "thinking":
			if text, ok := block["thinking"].(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, "[previous reasoning]\n"+text)
			}
		}
	}
	return strings.Join(out, "\n")
}

type doubaoMessage struct {
	ContentType int
	Text        string
	Think       string
	EventType   int
	Status      int
	Finished    bool
	Failure     string
}

func parseDoubaoEvent(data []byte) (doubaoMessage, bool) {
	var msg doubaoMessage
	var outer struct {
		EventType int             `json:"event_type"`
		EventData json.RawMessage `json:"event_data"`
	}
	if json.Unmarshal(data, &outer) != nil {
		return msg, false
	}
	msg.EventType = outer.EventType
	if outer.EventType == 2005 {
		var detail string
		if json.Unmarshal(outer.EventData, &detail) == nil {
			msg.Failure = detail
		} else {
			msg.Failure = string(outer.EventData)
		}
		msg.Finished = true
		return msg, true
	}
	var eventData map[string]any
	if json.Unmarshal(outer.EventData, &eventData) != nil {
		var inner string
		if json.Unmarshal(outer.EventData, &inner) != nil || json.Unmarshal([]byte(inner), &eventData) != nil {
			return msg, false
		}
	}
	message, _ := eventData["message"].(map[string]any)
	if message == nil {
		if raw, ok := eventData["message"].(string); ok {
			_ = json.Unmarshal([]byte(raw), &message)
		}
	}
	if message == nil {
		return msg, false
	}
	msg.Status = asInt(eventData["status"])
	msg.ContentType = asInt(message["content_type"])
	content := map[string]any{}
	switch value := message["content"].(type) {
	case map[string]any:
		content = value
	case string:
		_ = json.Unmarshal([]byte(value), &content)
	}
	switch msg.ContentType {
	case 2001, 2003, 10000:
		msg.Text, _ = content["text"].(string)
	case 2008:
		msg.Think, _ = content["think"].(string)
		msg.Text, _ = content["text"].(string)
	case 10040:
		msg.Think, _ = content["think"].(string)
		if msg.Think == "" {
			msg.Think, _ = content["title"].(string)
		}
	}
	if outer.EventType == 2003 {
		msg.Finished = true
	}
	return msg, true
}

func collectDoubao(body []byte) (string, string, error) {
	var text, think strings.Builder
	for _, block := range strings.Split(string(body), "\n\n") {
		payload := sseData(block)
		if payload == "" {
			continue
		}
		msg, ok := parseDoubaoEvent([]byte(payload))
		if !ok {
			continue
		}
		if msg.Failure != "" {
			return "", "", errors.New(sanitizeUpstream(msg.Failure))
		}
		text.WriteString(msg.Text)
		think.WriteString(msg.Think)
	}
	if text.Len() == 0 && think.Len() == 0 {
		return "", "", errors.New("Doubao returned no answer")
	}
	return text.String(), think.String(), nil
}

func sseData(block string) string {
	var data []string
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	return strings.Join(data, "\n")
}

func sanitizeUpstream(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) > 300 {
		raw = raw[:300]
	}
	if raw == "" {
		raw = "upstream request failed"
	}
	return raw
}

func mustJSON(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}

func randomUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func newMessageID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "msg_" + hex.EncodeToString(b)
}

func decodeBase64Loose(raw string) []byte {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	b, _ := base64.StdEncoding.DecodeString(raw)
	return b
}

func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		value, _ := n.Int64()
		return int(value)
	default:
		return 0
	}
}

func samanthaPayload(prompt string, deep bool, botID string) map[string]any {
	return map[string]any{
		"messages": []any{map[string]any{
			"content":      string(mustJSON(map[string]any{"text": prompt})),
			"content_type": 2001,
			"attachments":  []any{},
			"references":   []any{},
		}},
		"completion_option": map[string]any{
			"is_regen":                 false,
			"with_suggest":             false,
			"need_create_conversation": true,
			"launch_stage":             1,
			"is_replace":               false,
			"is_delete":                false,
			"is_ai_playground":         false,
			"memory_type":              2,
			"message_from":             0,
			"use_deep_think":           deep,
			"use_auto_cot":             deep,
			"resend_for_regen":         false,
			"enable_commerce_credit":   false,
		},
		"evaluate_option":       map[string]any{"web_ab_params": ""},
		"local_conversation_id": randomUUID(),
		"local_message_id":      randomUUID(),
		"bot_id":                botID,
	}
}

func chatCompletionPayload(prompt string, deep bool, fp string, botID string) map[string]any {
	textBlock := map[string]any{
		"block_type": 10000,
		"content": map[string]any{
			"text_block":     map[string]any{"text": prompt, "icon_url": "", "icon_url_dark": "", "summary": ""},
			"pc_event_block": "",
		},
		"block_id":      randomUUID(),
		"parent_id":     "",
		"meta_info":     []any{},
		"append_fields": []any{},
		"is_finish":     true,
		"patch_type":    2,
	}
	return map[string]any{
		"client_meta": map[string]any{
			"local_conversation_id": "local_" + strings.ReplaceAll(randomUUID(), "-", "")[:16],
			"conversation_id":       "",
			"bot_id":                botID,
			"last_section_id":       "",
			"last_message_index":    0,
		},
		"messages": []any{map[string]any{
			"local_message_id": randomUUID(),
			"content_block":    []any{textBlock},
			"message_status":   0,
		}},
		"option": map[string]any{
			"send_message_scene":       "",
			"create_time_ms":           0,
			"collect_id":               "",
			"is_audio":                 false,
			"answer_with_suggest":      false,
			"tts_switch":               false,
			"need_deep_think":          btoi(deep),
			"click_clear_context":      false,
			"from_suggest":             false,
			"is_regen":                 false,
			"is_replace":               false,
			"disable_sse_cache":        false,
			"select_text_action":       "",
			"resend_for_regen":         false,
			"scene_type":               0,
			"unique_key":               randomUUID(),
			"start_seq":                0,
			"need_create_conversation": true,
			"conversation_init_option": map[string]any{"need_ack_conversation": true},
			"regen_query_id":           []any{},
			"edit_query_id":            []any{},
			"regen_instruction":        "",
			"no_replace_for_regen":     false,
			"message_from":             0,
			"shared_app_name":          "",
			"action_bar_skill_id":      0,
			"sse_recv_event_options":   map[string]any{"support_chunk_delta": true},
			"is_ai_playground":         false,
		},
		"chat_ability": map[string]any{},
		"ext": map[string]any{
			"use_deep_think":                btoi(deep),
			"fp":                            fp,
			"use_submit_pipeline":           "1",
			"commerce_credit_config_enable": "0",
			"sub_conv_firstmet_type":        "1",
		},
	}
}

func btoi(v bool) int {
	if v {
		return 1
	}
	return 0
}

func parseChatEvent(data []byte, inThinking *bool) (doubaoMessage, bool) {
	var msg doubaoMessage
	var obj map[string]any
	if json.Unmarshal(data, &obj) != nil {
		return msg, false
	}
	if code := asInt(obj["error_code"]); code != 0 {
		msg.Failure = sanitizeUpstream(fmt.Sprintf("code=%d %v", code, obj["error_msg"]))
		msg.Finished = true
		return msg, true
	}
	if text, ok := obj["text"].(string); ok {
		if *inThinking {
			msg.Think = text
		} else {
			msg.Text = text
		}
		return msg, true
	}
	var blocks []any
	if content, ok := obj["content"].(map[string]any); ok {
		blocks = append(blocks, toSlice(content["content_block"])...)
	}
	for _, patch := range toSlice(obj["patch_op"]) {
		patchMap, _ := patch.(map[string]any)
		value, _ := patchMap["patch_value"].(map[string]any)
		blocks = append(blocks, toSlice(value["content_block"])...)
		if raw, ok := value["content"].(string); ok {
			var inner map[string]any
			if json.Unmarshal([]byte(raw), &inner) == nil {
				if text, ok := inner["text"].(string); ok && text != "" {
					if *inThinking {
						msg.Think += text
					} else {
						msg.Text += text
					}
				}
			}
		}
	}
	for _, raw := range blocks {
		block, _ := raw.(map[string]any)
		switch asInt(block["block_type"]) {
		case 10040:
			*inThinking = true
		case 10000:
			content, _ := block["content"].(map[string]any)
			textBlock, _ := content["text_block"].(map[string]any)
			text, _ := textBlock["text"].(string)
			if text == "" {
				continue
			}
			if *inThinking {
				msg.Think += text
			} else {
				msg.Text += text
			}
		}
	}
	return msg, true
}

func toSlice(v any) []any {
	if list, ok := v.([]any); ok {
		return list
	}
	return nil
}

func collectDoubaoChat(body []byte) (string, string, error) {
	var text, think strings.Builder
	inThinking := false
	for _, block := range strings.Split(string(body), "\n\n") {
		payload := sseData(block)
		if payload == "" {
			continue
		}
		msg, ok := parseChatEvent([]byte(payload), &inThinking)
		if !ok {
			continue
		}
		if msg.Failure != "" {
			return "", "", errors.New(msg.Failure)
		}
		text.WriteString(msg.Text)
		think.WriteString(msg.Think)
	}
	if text.Len() == 0 && think.Len() == 0 {
		return "", "", errors.New("Doubao returned no answer")
	}
	return text.String(), think.String(), nil
}
