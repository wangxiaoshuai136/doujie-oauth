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
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

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
	requestedModel := model
	baseModel, suffixEffort := splitModelAndEffort(model)
	baseModel = cleanModelID(baseModel)
	if baseModel == "" {
		baseModel = "doubao"
		requestedModel = baseModel
	}
	selection, hasSelection := selectionForRequest(c, baseModel, sender)
	if c.site() != siteCN {
		hasSelection = false
	}
	if c.site() == siteCN && !hasSelection {
		return out, pluginabi.NewError("model_unavailable", "model is not available for this Doubao account", 400)
	}
	promptLimit := 0
	if hasSelection {
		promptLimit = int(selection.Context)
	}
	extraction, err := extractPromptRequest(format, req.Payload, promptLimit)
	if err != nil {
		return out, pluginabi.NewError("invalid_request", err.Error(), 400)
	}
	prompt := extraction.Prompt
	chars := extraction.Chars
	requestEffort := extraction.Effort
	if strings.TrimSpace(suffixEffort) != "" {
		requestEffort = suffixEffort
	}
	requestDeep := extraction.Deep || reasoningEffortEnabled(suffixEffort)
	cfg := getConfig()
	profile := c.profile()
	deep := cfg.DeepThink || wantsDeepThink(baseModel) || requestDeep
	deepValue := "0"
	if hasSelection {
		selection.ReasoningEffort = selection.effectiveEffort(requestEffort)
		if selection.ReasoningEffort <= 0 {
			return out, pluginabi.NewError("model_catalog_error", "model has no reasoning effort level", http.StatusBadGateway)
		}
		deepValue = selection.ItemID
		if selection.ItemID != "0" {
			deep = true
		}
	}
	if cfg.DeepThink || wantsDeepThink(baseModel) || requestDeep {
		if deepValue == "0" || deepValue == "" {
			deepValue = "1"
		}
		deep = true
	}
	var selected *modelSelection
	if hasSelection {
		selected = &selection
	}
	params := securityParams(c, baseModel)
	endpoint := cfg.Endpoint
	if hasSelection {
		endpoint = "chat"
	} else if endpoint != "chat" {
		endpoint = "samantha"
	}
	var path string
	var sent map[string]any
	if endpoint == "chat" {
		path = "/chat/completion?"
		sent = chatCompletionPayload(prompt, c.botIDForModel(baseModel), c, selected, deepValue)
	} else {
		path = "/samantha/chat/completion?"
		sent = samanthaPayload(prompt, deep, c.botIDForModel(baseModel), selected, deepValue)
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
	return preparedRequest{HTTP: httpReq, Model: requestedModel, AuthID: req.AuthID, DeepThink: deep, InputChars: chars, Endpoint: endpoint, Format: format}, nil
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
	return params
}

func flattenConversation(body anthropicRequest, maxChars int) (string, int, error) {
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
	chars := utf8.RuneCountInString(prompt)
	if maxChars > 0 && chars > maxChars {
		return "", 0, errors.New("prompt exceeds the Doubao web context limit")
	}
	return prompt, chars, nil
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
	Err         *doubaoStreamError
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
		if obj := objectFromRaw(outer.EventData); len(obj) > 0 {
			if streamErr := streamErrorFromObject(obj); streamErr != nil {
				msg.Err = streamErr
				msg.Failure = streamErr.Error()
				msg.Finished = true
				return msg, true
			}
		}
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
		if msg.Err != nil {
			return "", "", msg.Err
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
	raw = upstreamURLPattern.ReplaceAllStringFunc(raw, redactURL)
	if len(raw) > 300 {
		raw = raw[:300]
	}
	if raw == "" {
		raw = "upstream request failed"
	}
	return raw
}

func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "[redacted-url]"
	}
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	u.User = nil
	return u.String()
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

func samanthaPayload(prompt string, deep bool, botID string, selection *modelSelection, deepValue string) map[string]any {
	message := map[string]any{
		"content":      string(mustJSON(map[string]any{"text": prompt})),
		"content_type": 2001,
		"attachments":  []any{},
		"references":   []any{},
	}
	if ext := samanthaModelExt(selection, deepValue); ext != nil {
		message["ext"] = ext
	}
	return map[string]any{
		"messages": []any{message},
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
			"use_auto_cot":             (selection == nil && deep) || deepValue == "2",
			"resend_for_regen":         false,
			"enable_commerce_credit":   false,
		},
		"evaluate_option":       map[string]any{"web_ab_params": ""},
		"local_conversation_id": randomUUID(),
		"local_message_id":      randomUUID(),
		"bot_id":                botID,
	}
}

func chatCompletionPayload(prompt string, botID string, c credential, selection *modelSelection, deepValue string) map[string]any {
	textBlock := map[string]any{
		"block_type": 10000,
		"content": map[string]any{
			"text_block":     map[string]any{"text": prompt, "icon_url": "", "icon_url_dark": "", "summary": ""},
			"pc_event_block": "",
		},
		"block_id":      randomUUID(),
		"parent_id":     "",
		"meta_info":     []any{},
		"extra_info":    map[string]any{},
		"append_fields": []any{},
		"is_finish":     true,
		"patch_type":    2,
	}
	now := time.Now()
	localMessageID := randomUUID()
	conversationInitOption := map[string]any{
		"project_id":            "",
		"need_ack_conversation": true,
	}
	agentMode := 2
	conversationMode := 1
	if selection != nil {
		agentMode = selection.AgentMode
		if parsed, err := strconv.Atoi(selection.ConversationMode); err == nil {
			conversationMode = parsed
		}
	}
	workMode := selection != nil && selection.isWorkMode()
	ext := map[string]any{
		"use_deep_think":                deepValue,
		"agent_mode":                    strconv.Itoa(agentMode),
		"conversation_init_option":      string(mustJSON(conversationInitOption)),
		"sub_conv_firstmet_type":        "1",
		"collection_id":                 "",
		"is_finish":                     "1",
		"commerce_credit_config_enable": "0",
	}
	option := map[string]any{
		"send_message_scene":        "",
		"create_time_ms":            now.UnixMilli(),
		"collect_id":                "",
		"is_audio":                  false,
		"answer_with_suggest":       false,
		"tts_switch":                false,
		"need_deep_think":           deepThinkValue(selection, deepValue),
		"agent_mode":                agentMode,
		"click_clear_context":       false,
		"from_suggest":              false,
		"is_regen":                  false,
		"is_replace":                false,
		"is_from_click_option":      false,
		"is_from_click_softlink":    false,
		"disable_sse_cache":         false,
		"select_text_action":        "",
		"is_select_text":            false,
		"resend_for_regen":          false,
		"scene_type":                0,
		"unique_key":                randomUUID(),
		"start_seq":                 0,
		"need_create_conversation":  true,
		"conversation_init_option":  conversationInitOption,
		"regen_query_id":            []any{},
		"edit_query_id":             []any{},
		"regen_instruction":         "",
		"no_replace_for_regen":      false,
		"message_from":              0,
		"shared_app_name":           "",
		"shared_app_id":             "",
		"sse_recv_event_options":    map[string]any{"support_chunk_delta": true},
		"support_lazy_fetch_stream": true,
		"is_ai_playground":          false,
		"is_old_user":               true,
		"recovery_option": map[string]any{
			"is_recovery":            false,
			"req_create_time_sec":    now.Unix(),
			"append_sse_event_scene": 0,
		},
		"message_storage_type":        0,
		"related_deleted_message_ids": map[string]any{},
		"connector_info_list":         []any{},
		"conversation_mode":           conversationMode,
	}
	var agentTaskParam map[string]any
	var generalTaskParam map[string]any
	if workMode {
		agentTaskParam = workModeAgentTaskParam(c)
		generalTaskParam = map[string]any{
			"action":                  0,
			"thread_local_message_id": []any{localMessageID},
			"project_id":              "",
			"runtime_type":            agentTaskParam["runtime_type"],
			"agent_task_param":        agentTaskParam,
			"server_do_compare":       true,
		}
		if _, local := agentTaskParam["sandbox_auth_type"]; local {
			generalTaskParam["client_option"] = workModeClientOption(c, agentTaskParam)
		}
		ext["general_task_param"] = string(mustJSON(generalTaskParam))
		option["general_task_param"] = generalTaskParam
	}
	if selection != nil {
		initExt := map[string]any{
			"model_item_key":   selection.ItemKey,
			"reasoning_effort": strconv.Itoa(selection.ReasoningEffort),
			"mode_id":          selection.ModeID,
		}
		if workMode {
			initExt["agent_task_param"] = string(mustJSON(agentTaskParam))
		}
		option["conversation_init_ext"] = initExt
		option["model_config"] = map[string]any{
			"model_item_key":     selection.ItemKey,
			"model_extra_params": selection.ExtraParams,
			"reasoning_effort":   selection.ReasoningEffort,
		}
		option["aggregate_params"] = map[string]any{
			"mention_skill_list":  "[]",
			"mention_plugin_list": "[]",
			"mention_ext":         "[{}]",
			"conversation_mode":   selection.ConversationMode,
			"mode_id":             selection.ModeID,
			"model_item_key":      selection.ItemKey,
			"agent_mode":          strconv.Itoa(selection.AgentMode),
			"reasoning_effort":    strconv.Itoa(selection.ReasoningEffort),
			"provider_id":         "",
		}
	}
	return map[string]any{
		"client_meta": map[string]any{
			"local_conversation_id": "local_" + strings.ReplaceAll(randomUUID(), "-", "")[:16],
			"conversation_id":       "",
			"bot_id":                botID,
			"last_section_id":       "",
			"last_message_index":    0,
			"local_permissions": []any{
				map[string]any{"permission_name": "ACCESS_COARSE_LOCATION", "status": 3},
				map[string]any{"permission_name": "ACCESS_FINE_LOCATION", "status": 3},
				map[string]any{"permission_name": "ACCESS_BACKGROUND_LOCATION", "status": 3},
			},
		},
		"messages": []any{map[string]any{
			"local_message_id": localMessageID,
			"content_block":    []any{textBlock},
			"message_status":   0,
		}},
		"option":       option,
		"user_context": []any{},
		"ext":          ext,
	}
}

// workModeAgentTaskParam mirrors the desktop client's agent task parameter.
// Cloud tasks carry only runtime_type=1; local tasks must carry the same
// device/workspace fields the client fills in before sending.
func workModeAgentTaskParam(c credential) map[string]any {
	cfg := getConfig()
	runtimeMode := strings.ToLower(strings.TrimSpace(cfg.Runtime))
	local := runtimeMode == "local" || (runtimeMode == "auto" && cfg.Workspace != "")
	if !local {
		return map[string]any{"runtime_type": 1}
	}
	workspace := strings.TrimSpace(cfg.Workspace)
	deviceName := strings.TrimSpace(cfg.DeviceName)
	if deviceName == "" {
		if host, err := os.Hostname(); err == nil {
			deviceName = strings.TrimSpace(host)
		}
	}
	folderName := ""
	if workspace != "" {
		folderName = workspaceFolderName(workspace)
	}
	localAppID := strings.TrimSpace(cfg.LocalAppID)
	if localAppID == "" {
		localAppID = c.profile().ChatAID
	}
	param := map[string]any{
		"runtime_type":      2,
		"device_name":       deviceName,
		"folder_name":       folderName,
		"local_app_id":      localAppID,
		"local_device_id":   c.DeviceID,
		"workspace":         workspace,
		"sandbox_auth_type": sandboxAuthTypeValue(cfg.SandboxAuthType),
	}
	return param
}

func workspaceFolderName(workspace string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(workspace), "\\", "/")
	normalized = strings.TrimRight(normalized, "/")
	if index := strings.LastIndex(normalized, "/"); index >= 0 {
		return normalized[index+1:]
	}
	return normalized
}

func sandboxAuthTypeValue(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ask_always":
		return 0
	case "always_allow":
		return 2
	default:
		return 1
	}
}

func workModeClientOption(c credential, agentTaskParam map[string]any) map[string]any {
	workspace, _ := agentTaskParam["workspace"].(string)
	authType, _ := agentTaskParam["sandbox_auth_type"].(int)
	osName := strings.TrimSpace(getConfig().ClientOS)
	if osName == "" {
		osName = "Windows"
	}
	return map[string]any{
		"agent_workspace": map[string]any{
			"agent_workspace":   workspace,
			"local_skill_paths": []any{},
		},
		"client_env_id":      randomUUID(),
		"enable_sandbox":     true,
		"os":                 osName,
		"sandbox_auth_type":  authType,
		"sandbox_env_id":     randomUUID(),
		"sandbox_id":         "deferred-sandbox-" + randomUUID(),
		"shared_folder_path": []any{workspace},
		"workspace":          workspace,
	}
}

func samanthaModelExt(selection *modelSelection, deepValue string) map[string]any {
	if selection == nil {
		return nil
	}
	return map[string]any{
		"model_item_key":     selection.ItemKey,
		"mode_id":            selection.ModeID,
		"agent_mode":         strconv.Itoa(selection.AgentMode),
		"model_extra_params": string(mustJSON(selection.ExtraParams)),
		"reasoning_effort":   strconv.Itoa(selection.ReasoningEffort),
		"use_deep_think":     deepValue,
	}
}

func deepThinkValue(selection *modelSelection, deepValue string) int {
	if selection == nil {
		return btoi(deepValue != "0")
	}
	value, err := strconv.Atoi(selection.ItemID)
	if err != nil || value < 0 {
		return 0
	}
	return value
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
	if code := looseInt(obj["error_code"]); code != 0 {
		msg.Err = newDoubaoStreamError(code, firstStringValue(obj["error_msg"], obj["message"], obj["msg"]), obj)
		msg.Failure = msg.Err.Error()
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
		if raw, ok := content["content"].(string); ok && raw != "" {
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
			*inThinking = false
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
		if msg.Err != nil {
			return "", "", msg.Err
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
