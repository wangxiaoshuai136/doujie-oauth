package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const (
	videoModelID      = "doubao-video"
	videoAgentBotID   = "7338286299411103781"
	defaultVideoRatio = "16:9"
)

var durationPattern = regexp.MustCompile(`(?i)(\d{1,2})\s*(秒|s|sec|seconds)`)

type generatedVideo struct {
	URL      string
	CoverURL string
	Width    int
	Height   int
	Duration float64
}

type doubaoVideoResult struct {
	TaskID           string
	Vid              string
	Model            string
	ModelValue       string
	Remain           int
	ConversationID   string
	SectionID        string
	LastMessageIndex int
	Videos           []generatedVideo
	Text             string
	Plan             bool
}

type videoModelOption struct {
	Value           string `json:"value"`
	ShowName        string `json:"show_name"`
	Default         bool   `json:"is_default"`
	SubscriptionTag string `json:"subscription_tag"`
}

type videoSkillPack struct {
	Models       []videoModelOption
	DefaultModel videoModelOption
}

var fallbackVideoModelValues = map[string]string{
	"seedance-2.0":      "seedance_v2.0_std",
	"seedance-2.0-fast": "seedance_v2.0",
	"seedance-2.0-mini": "seedance_v2.0_mini",
}

func isVideoModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "doubao-video" || model == "doubao-creative-video" || model == "doubao-session-video" {
		return true
	}
	if model == "seedance" {
		return true
	}
	return strings.HasPrefix(model, "seedance-") || strings.HasPrefix(model, "seedance_")
}

func canonicalVideoModel(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "doubao-session-video" || model == "seedance" || model == "" {
		return videoModelID
	}
	return model
}

func videoModelLabel(model string) string {
	switch canonicalVideoModel(model) {
	case videoModelID:
		return "豆包自动选择可用视频模型"
	case "seedance-2.5", "seedance_2.5":
		return "Seedance 2.5"
	case "seedance-2.0", "seedance_2.0":
		return "Seedance 2.0"
	case "seedance-2.0-fast", "seedance_2.0_fast":
		return "Seedance 2.0 Fast"
	case "seedance-2.0-mini", "seedance_2.0_mini":
		return "Seedance 2.0 Mini"
	case "seedance_v2.5":
		return "Seedance 2.5"
	case "seedance_v2.0_std":
		return "Seedance 2.0"
	case "seedance_v2.0":
		return "Seedance 2.0 Fast"
	case "seedance_v2.0_mini":
		return "Seedance 2.0 Mini"
	default:
		return strings.TrimSpace(model)
	}
}

func buildVideoPayload(prompt, ratio, duration, modelValue, modelLabel, resolution string) map[string]any {
	prompt = strings.TrimSpace(prompt)
	ratio = strings.TrimSpace(ratio)
	if ratio == "" {
		ratio = "auto"
	}
	resolution = strings.TrimSpace(resolution)
	if resolution == "" {
		resolution = "720p"
	}
	seconds := 10
	if value, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(duration, "s"))); err == nil {
		seconds = value
	} else if match := durationPattern.FindStringSubmatch(duration); len(match) > 1 {
		if value, convErr := strconv.Atoi(match[1]); convErr == nil {
			seconds = value
		}
	}
	if seconds < 4 || seconds > 15 {
		seconds = 10
	}
	if strings.TrimSpace(modelValue) == "" {
		modelValue = fallbackVideoModelValues[canonicalVideoModel(modelLabel)]
	}
	if strings.TrimSpace(modelValue) == "" {
		modelValue = "seedance_v2.0_mini"
	}
	text := fmt.Sprintf("生成视频：%s，%ds，%s", prompt, seconds, resolution)
	abilityParam := string(mustJSON(map[string]any{
		"ratio":      ratio,
		"model":      modelValue,
		"duration":   seconds,
		"resolution": resolution,
		"input_box_content": map[string]any{
			"user_input_content":   prompt,
			"reply_message_format": "生成视频：%s",
		},
	}))

	messageID := randomUUID()
	localConversationID := "local_" + strings.ReplaceAll(randomUUID(), "-", "")[:16]
	now := time.Now()
	return map[string]any{
		"client_meta": map[string]any{
			"local_conversation_id": localConversationID,
			"conversation_id":       "",
			"bot_id":                videoAgentBotID,
			"last_section_id":       "",
			"last_message_index":    nil,
			"local_permissions": []any{
				map[string]any{"permission_name": "ACCESS_COARSE_LOCATION", "status": 3},
				map[string]any{"permission_name": "ACCESS_FINE_LOCATION", "status": 3},
				map[string]any{"permission_name": "ACCESS_BACKGROUND_LOCATION", "status": 3},
			},
		},
		"messages": []any{map[string]any{
			"local_message_id": messageID,
			"content_block": []any{map[string]any{
				"block_type": 10000,
				"content": map[string]any{
					"text_block": map[string]any{
						"text":          text,
						"icon_url":      "",
						"icon_url_dark": "",
						"summary":       "",
					},
					"pc_event_block": "",
				},
				"block_id":      randomUUID(),
				"parent_id":     "",
				"meta_info":     []any{},
				"extra_info":    map[string]any{},
				"append_fields": []any{},
			}},
			"message_status": 0,
		}},
		"option": map[string]any{
			"send_message_scene":        "",
			"create_time_ms":            now.UnixMilli(),
			"collect_id":                "",
			"is_audio":                  false,
			"answer_with_suggest":       false,
			"tts_switch":                false,
			"need_deep_think":           0,
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
			"conversation_init_option":  map[string]any{"need_ack_conversation": true},
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
			"general_task_param":        map[string]any{"server_do_compare": true},
			"recovery_option": map[string]any{
				"is_recovery":            false,
				"req_create_time_sec":    now.Unix(),
				"append_sse_event_scene": 0,
			},
			"message_storage_type":        0,
			"related_deleted_message_ids": map[string]any{},
			"connector_info_list":         []any{},
			"model_config":                map[string]any{"model_item_key": "", "model_extra_params": map[string]any{}},
			"aggregate_params": map[string]any{
				"mention_skill_list":  "[]",
				"mention_plugin_list": "[]",
				"mention_ext":         "[{}]",
				"conversation_mode":   "1",
				"mode_id":             "",
				"model_item_key":      "",
				"agent_mode":          "",
				"reasoning_effort":    "",
				"provider_id":         "",
			},
			"conversation_mode": 1,
		},
		"chat_ability": map[string]any{
			"ability_type":  17,
			"ability_param": abilityParam,
		},
		"user_context": []any{},
		"ext": map[string]any{
			"answer_with_suggest":           "0",
			"sub_conv_firstmet_type":        "1",
			"general_task_param":            `{"server_do_compare":true}`,
			"collection_id":                 "",
			"is_finish":                     "1",
			"conversation_init_option":      `{"need_ack_conversation":true}`,
			"commerce_credit_config_enable": "0",
		},
	}
}

func buildVideoConfirmPayload(conversationID, sectionID string, lastMessageIndex int) map[string]any {
	messageID := randomUUID()
	now := time.Now()
	if lastMessageIndex < 0 {
		lastMessageIndex = 0
	}
	return map[string]any{
		"client_meta": map[string]any{
			"conversation_id":    conversationID,
			"bot_id":             videoAgentBotID,
			"last_section_id":    sectionID,
			"last_message_index": lastMessageIndex,
			"local_permissions": []any{
				map[string]any{"permission_name": "ACCESS_COARSE_LOCATION", "status": 3},
				map[string]any{"permission_name": "ACCESS_FINE_LOCATION", "status": 3},
				map[string]any{"permission_name": "ACCESS_BACKGROUND_LOCATION", "status": 3},
			},
		},
		"messages": []any{map[string]any{
			"local_message_id": messageID,
			"content_block": []any{map[string]any{
				"block_type": 10000,
				"content": map[string]any{
					"text_block": map[string]any{
						"text":          "确认生成",
						"icon_url":      "",
						"icon_url_dark": "",
						"summary":       "",
					},
					"pc_event_block": "",
				},
				"block_id":      randomUUID(),
				"parent_id":     "",
				"meta_info":     []any{},
				"extra_info":    map[string]any{},
				"append_fields": []any{},
			}},
			"message_status": 0,
		}},
		"option": map[string]any{
			"send_message_scene":        "",
			"create_time_ms":            now.UnixMilli(),
			"collect_id":                "",
			"is_audio":                  false,
			"answer_with_suggest":       false,
			"agent_mode":                2,
			"tts_switch":                false,
			"need_deep_think":           0,
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
			"need_create_conversation":  false,
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
			"general_task_param":        map[string]any{"runtime_type": 0, "agent_task_param": map[string]any{}, "project_id": "", "server_do_compare": true},
			"recovery_option": map[string]any{
				"is_recovery":            false,
				"req_create_time_sec":    now.Unix(),
				"append_sse_event_scene": 0,
			},
			"message_storage_type":        0,
			"related_deleted_message_ids": map[string]any{},
			"connector_info_list":         []any{},
			"model_config":                map[string]any{"model_item_key": "0", "model_extra_params": map[string]any{}, "reasoning_effort": 5},
			"aggregate_params": map[string]any{
				"mention_skill_list":  "[]",
				"mention_plugin_list": "[]",
				"mention_ext":         "[{}]",
				"conversation_mode":   "",
				"mode_id":             "1",
				"model_item_key":      "0",
				"agent_mode":          "2",
				"reasoning_effort":    "5",
				"provider_id":         "",
			},
		},
		"user_context": []any{},
		"ext": map[string]any{
			"agent_mode":                    "2",
			"use_deep_think":                "0",
			"general_task_param":            `{"runtime_type":0,"agent_task_param":{},"project_id":"","server_do_compare":true}`,
			"collection_id":                 "",
			"is_finish":                     "1",
			"commerce_credit_config_enable": "0",
		},
	}
}

func extractVideoRequest(format string, payload []byte) (string, bool, error) {
	var root struct {
		Prompt    string `json:"prompt"`
		Confirm   any    `json:"confirm"`
		Confirmed any    `json:"confirmed"`
	}
	_ = json.Unmarshal(payload, &root)
	explicit := false
	for _, value := range []any{root.Confirm, root.Confirmed} {
		switch item := value.(type) {
		case bool:
			explicit = explicit || item
		case string:
			explicit = explicit || strings.EqualFold(strings.TrimSpace(item), "true") || item == "1"
		case float64:
			explicit = explicit || item != 0
		}
	}

	texts := videoUserTexts(format, payload)
	if len(texts) == 0 && strings.TrimSpace(root.Prompt) != "" {
		texts = append(texts, strings.TrimSpace(root.Prompt))
	}
	if len(texts) == 0 {
		return "", false, fmt.Errorf("a user message with the video prompt is required")
	}
	last := texts[len(texts)-1]
	prompt := last
	confirmed := explicit || isExplicitVideoConfirmation(last)
	if isShortVideoConfirmation(last) && len(texts) >= 2 {
		prompt = texts[len(texts)-2]
		confirmed = true
	}
	if utf8.RuneCountInString(prompt) > maxPromptChars {
		return "", false, fmt.Errorf("video prompt exceeds the Doubao context limit")
	}
	return prompt, confirmed, nil
}

func videoUserTexts(format string, payload []byte) []string {
	var texts []string
	switch format {
	case "openai":
		var body openAIChatRequest
		if json.Unmarshal(payload, &body) != nil {
			return texts
		}
		for _, message := range body.Messages {
			if !strings.EqualFold(strings.TrimSpace(message.Role), "user") {
				continue
			}
			if text := strings.TrimSpace(flattenBlocks(message.Content)); text != "" {
				texts = append(texts, text)
			}
		}
	case "openai-response":
		var body responsesRequest
		if json.Unmarshal(payload, &body) != nil {
			return texts
		}
		if text, ok := rawText(body.Input); ok {
			if strings.TrimSpace(text) != "" {
				texts = append(texts, strings.TrimSpace(text))
			}
			return texts
		}
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
				if !strings.EqualFold(strings.TrimSpace(item.Role), "user") {
					continue
				}
				if text := strings.TrimSpace(flattenBlocks(item.Content)); text != "" {
					texts = append(texts, text)
				}
			}
		}
	default:
		var body anthropicRequest
		if json.Unmarshal(payload, &body) != nil {
			return texts
		}
		for _, message := range body.Messages {
			if !strings.EqualFold(strings.TrimSpace(message.Role), "user") {
				continue
			}
			if text := strings.TrimSpace(flattenBlocks(message.Content)); text != "" {
				texts = append(texts, text)
			}
		}
	}
	return texts
}

func isShortVideoConfirmation(text string) bool {
	text = strings.TrimSpace(strings.ToLower(text))
	if utf8.RuneCountInString(text) > 24 {
		return false
	}
	for _, marker := range []string{"确认", "生成", "开始", "可以", "好的", "ok", "yes"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func isExplicitVideoConfirmation(text string) bool {
	text = strings.ToLower(text)
	for _, marker := range []string{"已确认", "直接生成", "不要再询问", "无需确认", "开始生成", "确认执行"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func videoRatioFromPayload(payload []byte) (string, error) {
	var root struct {
		Ratio       string `json:"ratio"`
		AspectRatio string `json:"aspect_ratio"`
		Size        string `json:"size"`
	}
	_ = json.Unmarshal(payload, &root)
	ratio := firstNonEmpty(root.Ratio, root.AspectRatio, root.Size)
	if ratio == "" {
		return defaultVideoRatio, nil
	}
	ratio = strings.ToLower(strings.TrimSpace(ratio))
	if strings.Contains(ratio, "x") {
		parts := strings.SplitN(ratio, "x", 2)
		width, height := 0, 0
		_, _ = fmt.Sscanf(parts[0], "%d", &width)
		if len(parts) == 2 {
			_, _ = fmt.Sscanf(parts[1], "%d", &height)
		}
		switch {
		case width <= 0 || height <= 0:
			return "", fmt.Errorf("invalid video size %q", root.Size)
		case width > height:
			return "16:9", nil
		case height > width:
			return "9:16", nil
		default:
			return "1:1", nil
		}
	}
	switch ratio {
	case "1:1", "16:9", "9:16":
		return ratio, nil
	default:
		return "", fmt.Errorf("video ratio must be 1:1, 16:9, or 9:16")
	}
}

func videoDurationFromPayload(prompt string, payload []byte) string {
	var root struct {
		Seconds  any `json:"seconds"`
		Duration any `json:"duration"`
	}
	_ = json.Unmarshal(payload, &root)
	for _, value := range []any{root.Seconds, root.Duration} {
		if text := strings.TrimSpace(fmt.Sprintf("%v", value)); text != "" && text != "<nil>" && text != "0" {
			return text + " 秒"
		}
	}
	if match := durationPattern.FindStringSubmatch(prompt); len(match) > 0 {
		return match[1] + " 秒"
	}
	return ""
}

func videoAgentParams(c credential) url.Values {
	profile := c.profile()
	params := securityParams(c, videoModelID)
	params.Set("aid", profile.PassportAID)
	params.Set("real_aid", profile.PassportAID)
	params.Set("pc_version", profile.WebVersion)
	params.Set("doubao_pc_version", profile.WebVersion)
	params.Set("web_platform", "browser")
	params.Set("tea_uuid", c.WebID)
	params.Del("msToken")
	if profile.ID == siteCN {
		params.Set("tz_name", "Asia/Shanghai")
	} else {
		params.Set("tz_name", "UTC")
	}
	params.Del("chromium_version")
	params.Del("client_platform")
	params.Del("runtime")
	params.Del("runtime_version")
	if fp := strings.TrimSpace(c.Cookies["s_v_web_id"]); fp != "" {
		params.Set("fp", fp)
	}
	return params
}

func videoRequestHeaders(c credential, accept string) http.Header {
	headers := http.Header{
		"Accept":          {accept},
		"Content-Type":    {"application/json"},
		"Agw-Js-Conv":     {"str, str"},
		"User-Agent":      {chromeUA},
		"Referer":         {c.profile().Root + "/chat/"},
		"Origin":          {c.profile().Root},
		"Accept-Language": {c.profile().Language + ",en;q=0.8"},
		"Cookie":          {c.cookieHeader()},
	}
	csrf := strings.TrimSpace(c.Cookies["passport_csrf_token"])
	if csrf == "" {
		csrf = strings.TrimSpace(c.Cookies["passport_csrf_token_default"])
	}
	if csrf != "" {
		headers.Set("X-Tt-Passport-Csrf-Token", csrf)
	}
	return headers
}

func doVideoJSONRequest(c credential, path string, body []byte, sender httpSender) (pluginapi.HTTPResponse, error) {
	resp, err := sender.Do(pluginapi.HTTPRequest{
		Method:  http.MethodPost,
		URL:     c.profile().Root + path + "?" + videoAgentParams(c).Encode(),
		Headers: videoRequestHeaders(c, "application/json, text/plain, */*"),
		Body:    body,
	})
	if err != nil {
		return resp, pluginabi.NewError("transport_error", "Doubao video API transport failed", http.StatusServiceUnavailable)
	}
	if resp.StatusCode != http.StatusOK {
		return resp, upstreamError(resp.StatusCode, resp.Body)
	}
	return resp, nil
}

func fetchVideoSkillPack(c credential, sender httpSender) (videoSkillPack, error) {
	resp, err := doVideoJSONRequest(c, "/samantha/skill/pack", []byte(`{"skill_type":17}`), sender)
	if err != nil {
		return videoSkillPack{}, err
	}
	var root struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Msg     string `json:"msg"`
		Data    struct {
			VideoGeneration struct {
				Meta struct {
					OptionList []struct {
						Value   string             `json:"value"`
						Options []videoModelOption `json:"options"`
					} `json:"option_list"`
				} `json:"meta"`
			} `json:"video_generation"`
		} `json:"data"`
	}
	if json.Unmarshal(resp.Body, &root) != nil {
		return videoSkillPack{}, pluginabi.NewError("upstream_error", "Doubao video skill response was invalid", http.StatusBadGateway)
	}
	if root.Code != 0 {
		return videoSkillPack{}, pluginabi.NewError("upstream_rejected", sanitizeUpstream(firstNonEmpty(root.Message, root.Msg, "video skill unavailable")), http.StatusBadGateway)
	}
	pack := videoSkillPack{}
	for _, group := range root.Data.VideoGeneration.Meta.OptionList {
		if strings.EqualFold(strings.TrimSpace(group.Value), "model") {
			pack.Models = append(pack.Models, group.Options...)
		}
	}
	for _, option := range pack.Models {
		if option.Default {
			pack.DefaultModel = option
			break
		}
	}
	if pack.DefaultModel.Value == "" && len(pack.Models) > 0 {
		pack.DefaultModel = pack.Models[0]
	}
	return pack, nil
}

func videoResolutionFromPayload(payload []byte) string {
	var root struct {
		Resolution      string `json:"resolution"`
		VideoResolution string `json:"video_resolution"`
	}
	_ = json.Unmarshal(payload, &root)
	resolution := strings.ToLower(strings.TrimSpace(firstNonEmpty(root.Resolution, root.VideoResolution)))
	switch resolution {
	case "", "auto":
		return ""
	case "720":
		return "720p"
	case "1080":
		return "1080p"
	default:
		return resolution
	}
}

func videoCameraMovementFromPayload(payload []byte) string {
	var root struct {
		CameraMovement string `json:"camera_movement"`
	}
	_ = json.Unmarshal(payload, &root)
	return strings.TrimSpace(root.CameraMovement)
}

func resolveVideoModel(pack videoSkillPack, requested string) (videoModelOption, error) {
	requested = canonicalVideoModel(requested)
	if requested == videoModelID {
		if pack.DefaultModel.Value != "" {
			return pack.DefaultModel, nil
		}
		if len(pack.Models) > 0 {
			return pack.Models[0], nil
		}
		return videoModelOption{Value: "seedance_v2.0_mini", ShowName: "Seedance 2.0 Mini"}, nil
	}
	label := videoModelLabel(requested)
	fallbackValue := fallbackVideoModelValues[requested]
	for _, option := range pack.Models {
		if strings.EqualFold(strings.TrimSpace(option.ShowName), label) ||
			strings.EqualFold(strings.TrimSpace(option.Value), requested) ||
			(fallbackValue != "" && strings.EqualFold(strings.TrimSpace(option.Value), fallbackValue)) {
			return option, nil
		}
	}
	if fallbackValue != "" {
		return videoModelOption{Value: fallbackValue, ShowName: label}, nil
	}
	available := make([]string, 0, len(pack.Models))
	for _, option := range pack.Models {
		if option.ShowName != "" {
			available = append(available, option.ShowName)
		}
	}
	if len(available) == 0 {
		available = []string{"Seedance 2.0", "Seedance 2.0 Fast", "Seedance 2.0 Mini"}
	}
	return videoModelOption{}, fmt.Errorf("当前账号没有可用的 %s；可用模型：%s", label, strings.Join(available, "、"))
}

func fetchVideoGenInfo(c credential, sender httpSender) (int, bool, error) {
	resp, err := doVideoJSONRequest(c, "/samantha/video/query_video_gen_info", []byte(`{}`), sender)
	if err != nil {
		return 0, false, err
	}
	var root struct {
		Code int `json:"code"`
		Data struct {
			RemainCount       int  `json:"remain_count"`
			HasGeneratingTask bool `json:"has_generating_task"`
		} `json:"data"`
	}
	if json.Unmarshal(resp.Body, &root) != nil {
		return 0, false, pluginabi.NewError("upstream_error", "Doubao video quota response was invalid", http.StatusBadGateway)
	}
	if root.Code != 0 {
		return 0, false, upstreamFailureError("video quota query failed")
	}
	return root.Data.RemainCount, root.Data.HasGeneratingTask, nil
}

func resolveVideoPlayInfo(c credential, vid string, sender httpSender) ([]generatedVideo, error) {
	vid = strings.TrimSpace(vid)
	if vid == "" {
		return nil, nil
	}
	resp, err := doVideoJSONRequest(c, "/samantha/video/get_play_info", mustJSON(map[string]any{"vid": vid}), sender)
	if err != nil {
		return nil, err
	}
	var root any
	if json.Unmarshal(resp.Body, &root) != nil {
		return nil, pluginabi.NewError("upstream_error", "Doubao video play info was invalid", http.StatusBadGateway)
	}
	seen := map[string]struct{}{}
	videos := []generatedVideo{}
	collectVideoPlayInfo(root, &videos, seen)
	if len(videos) == 0 {
		return nil, pluginabi.NewError("upstream_error", "Doubao returned a video id without a playable URL", http.StatusBadGateway)
	}
	return videos, nil
}

func collectVideoPlayInfo(value any, videos *[]generatedVideo, seen map[string]struct{}) {
	switch item := value.(type) {
	case map[string]any:
		url := firstNonEmpty(
			videoValueString(item["main"]),
			videoValueString(item["main_url"]),
			videoValueString(item["url"]),
			videoValueString(item["backup"]),
		)
		if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
			if _, exists := seen[url]; !exists {
				seen[url] = struct{}{}
				*videos = append(*videos, generatedVideo{
					URL:      url,
					CoverURL: firstNonEmpty(videoValueString(item["cover"]), videoValueString(item["poster_url"])),
					Width:    asInt(item["width"]),
					Height:   asInt(item["height"]),
				})
			}
		}
		for _, child := range item {
			collectVideoPlayInfo(child, videos, seen)
		}
	case []any:
		for _, child := range item {
			collectVideoPlayInfo(child, videos, seen)
		}
	}
}

func videoValueString(value any) string {
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprintf("%v", value))
	if text == "<nil>" {
		return ""
	}
	return text
}

func doVideoRequest(c credential, params url.Values, body []byte, sender httpSender) (pluginapi.HTTPResponse, error) {
	headers := http.Header{
		"Accept":          {"text/event-stream"},
		"Content-Type":    {"application/json"},
		"Agw-Js-Conv":     {"str, str"},
		"User-Agent":      {chromeUA},
		"Referer":         {c.profile().Root + "/chat/"},
		"Origin":          {c.profile().Root},
		"Accept-Language": {c.profile().Language + ",en;q=0.8"},
		"Cookie":          {c.cookieHeader()},
	}
	csrf := strings.TrimSpace(c.Cookies["passport_csrf_token"])
	if csrf == "" {
		csrf = strings.TrimSpace(c.Cookies["passport_csrf_token_default"])
	}
	if csrf != "" {
		headers.Set("X-Tt-Passport-Csrf-Token", csrf)
	}
	resp, err := sender.Do(pluginapi.HTTPRequest{
		Method:  http.MethodPost,
		URL:     c.profile().Root + "/chat/completion?" + params.Encode(),
		Headers: headers,
		Body:    body,
	})
	if err != nil {
		return resp, pluginabi.NewError("transport_error", "Doubao video transport failed", http.StatusServiceUnavailable)
	}
	if resp.StatusCode != http.StatusOK {
		return resp, upstreamError(resp.StatusCode, resp.Body)
	}
	return resp, nil
}

func generateDoubaoAgentVideo(c credential, prompt, ratio, duration, modelValue, modelLabel, resolution string, sender httpSender) (doubaoVideoResult, error) {
	payload := buildVideoPayload(prompt, ratio, duration, modelValue, modelLabel, resolution)
	resp, err := doVideoRequest(c, videoAgentParams(c), mustJSON(payload), sender)
	if err != nil {
		return doubaoVideoResult{}, err
	}
	result, err := parseDoubaoAgentResponse(resp.Body)
	if err != nil {
		return result, err
	}
	if len(result.Videos) == 0 && result.Vid == "" && result.ConversationID != "" {
		confirm := buildVideoConfirmPayload(result.ConversationID, result.SectionID, result.LastMessageIndex)
		confirmResp, confirmErr := doVideoRequest(c, videoAgentParams(c), mustJSON(confirm), sender)
		if confirmErr != nil {
			return result, confirmErr
		}
		confirmed, confirmParseErr := parseDoubaoAgentResponse(confirmResp.Body)
		if confirmParseErr != nil {
			return result, confirmParseErr
		}
		for _, video := range confirmed.Videos {
			result.Videos = append(result.Videos, video)
		}
		if confirmed.Vid != "" {
			result.Vid = confirmed.Vid
		}
		if confirmed.TaskID != "" {
			result.TaskID = confirmed.TaskID
		}
		if confirmed.Text != "" {
			result.Text = confirmed.Text
		}
	}
	if len(result.Videos) == 0 && strings.TrimSpace(result.Vid) != "" {
		videos, playErr := resolveVideoPlayInfo(c, result.Vid, sender)
		if playErr == nil {
			result.Videos = videos
		} else if strings.TrimSpace(result.Text) == "" {
			return result, playErr
		}
	}
	return result, nil
}

func parseDoubaoAgentResponse(raw []byte) (doubaoVideoResult, error) {
	result := doubaoVideoResult{Videos: []generatedVideo{}}
	seen := map[string]struct{}{}
	texts := []string{}
	for _, block := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n\n") {
		if strings.TrimSpace(block) == "" {
			continue
		}
		eventName := ""
		dataLines := []string{}
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "event:") {
				eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			} else if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		payload := strings.Join(dataLines, "\n")
		if payload == "" {
			continue
		}
		if eventName == "gateway-error" {
			return result, videoGatewayError([]byte(payload))
		}
		var event map[string]any
		if json.Unmarshal([]byte(payload), &event) != nil {
			continue
		}
		if eventName == "STREAM_ERROR" {
			if code := asInt(event["error_code"]); code != 0 {
				if code == 710022002 || code == 710022004 || code == 710012001 {
					return result, pluginabi.NewError("reauth_required", "Doubao session or verification challenge rejected the video request", http.StatusUnauthorized)
				}
				return result, pluginabi.NewError("upstream_rejected", sanitizeUpstream(fmt.Sprintf("Doubao video error %d: %v", code, event["error_msg"])), http.StatusBadGateway)
			}
		}
		if eventName == "SSE_ACK" {
			ack := mapValue(event["ack_client_meta"])
			if value := strings.TrimSpace(fmt.Sprintf("%v", ack["conversation_id"])); value != "" && value != "<nil>" {
				result.ConversationID = value
			}
			if value := strings.TrimSpace(fmt.Sprintf("%v", firstNonEmpty(fmt.Sprintf("%v", ack["last_section_id"]), fmt.Sprintf("%v", ack["section_id"])))); value != "" && value != "<nil>" {
				result.SectionID = value
			}
			if index := asInt(ack["msg_cursor"]); index > 0 {
				result.LastMessageIndex = index
			} else if index := asInt(ack["latest_index"]); index > 0 {
				result.LastMessageIndex = index
			}
			if list, ok := event["query_list"].([]any); ok && len(list) > 0 {
				if index := asInt(mapValue(list[0])["message_index"]); index > result.LastMessageIndex {
					result.LastMessageIndex = index
				}
			}
		}
		allowText := eventName == "STREAM_CHUNK" || eventName == "STREAM_MSG_NOTIFY"
		collectAgentVideoValue(event, &result, seen, &texts, allowText)
	}
	result.Text = strings.Join(texts, "")
	return result, nil
}

func collectAgentVideoValue(value any, result *doubaoVideoResult, seen map[string]struct{}, texts *[]string, allowText bool) {
	switch item := value.(type) {
	case map[string]any:
		if allowText {
			if block := mapValue(item["text_block"]); len(block) > 0 {
				if text := strings.TrimSpace(fmt.Sprintf("%v", block["text"])); text != "" && text != "<nil>" {
					if len(*texts) == 0 || (*texts)[len(*texts)-1] != text {
						*texts = append(*texts, text)
					}
				}
			}
		}
		if modelValue, ok := item["video_model"]; ok {
			cover := firstNonEmpty(
				fmt.Sprintf("%v", item["poster_url"]),
				fmt.Sprintf("%v", item["cover_url"]),
				fmt.Sprintf("%v", mapValue(item["cover"])["url"]),
			)
			if cover == "<nil>" {
				cover = ""
			}
			for _, video := range videosFromModelValue(modelValue, cover) {
				if _, exists := seen[video.URL]; exists {
					continue
				}
				seen[video.URL] = struct{}{}
				result.Videos = append(result.Videos, video)
			}
		}
		if tools, ok := item["ai_creation_tool_list"]; ok {
			for _, tool := range agentToolList(tools) {
				toolMap := mapValue(tool)
				if taskID := strings.TrimSpace(fmt.Sprintf("%v", toolMap["task_id"])); taskID != "" && taskID != "<nil>" && result.TaskID == "" {
					result.TaskID = taskID
				}
			}
		}
		if vid := strings.TrimSpace(fmt.Sprintf("%v", item["vid"])); vid != "" && vid != "<nil>" && result.Vid == "" {
			result.Vid = vid
		}
		for _, child := range item {
			collectAgentVideoValue(child, result, seen, texts, allowText)
		}
	case []any:
		for _, child := range item {
			collectAgentVideoValue(child, result, seen, texts, allowText)
		}
	}
}

func agentToolList(value any) []any {
	switch item := value.(type) {
	case []any:
		return item
	case string:
		var list []any
		if json.Unmarshal([]byte(item), &list) == nil {
			return list
		}
	}
	return nil
}

func videosFromModelValue(value any, cover string) []generatedVideo {
	model := mapValue(value)
	if len(model) == 0 {
		return nil
	}
	var out []generatedVideo
	for _, raw := range mapValues(model["video_list"]) {
		info := mapValue(raw)
		mainURL := strings.TrimSpace(fmt.Sprintf("%v", info["main_url"]))
		if mainURL == "" || mainURL == "<nil>" {
			continue
		}
		if !strings.HasPrefix(mainURL, "http://") && !strings.HasPrefix(mainURL, "https://") {
			mainURL = strings.TrimSpace(string(decodeBase64Loose(mainURL)))
		}
		if !strings.HasPrefix(mainURL, "http://") && !strings.HasPrefix(mainURL, "https://") {
			continue
		}
		out = append(out, generatedVideo{
			URL:      mainURL,
			CoverURL: cover,
			Width:    asInt(info["vwidth"]),
			Height:   asInt(info["vheight"]),
			Duration: 0,
		})
	}
	return out
}

func videoGatewayError(payload []byte) error {
	var event struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
		Msg     string `json:"msg"`
	}
	if json.Unmarshal(payload, &event) == nil {
		message := firstNonEmpty(event.Message, event.Msg, string(payload))
		code := asInt(event.Code)
		if code == 401 || code == 403 || code == 710012001 || code == 710012002 {
			return pluginabi.NewError("reauth_required", "Doubao session expired or rejected; sign in again", http.StatusUnauthorized)
		}
		return pluginabi.NewError("upstream_rejected", "Doubao gateway rejected the video request: "+sanitizeUpstream(message), http.StatusBadGateway)
	}
	return pluginabi.NewError("upstream_rejected", "Doubao gateway rejected the video request", http.StatusBadGateway)
}

func mapValue(value any) map[string]any {
	switch item := value.(type) {
	case map[string]any:
		return item
	case string:
		var out map[string]any
		if json.Unmarshal([]byte(item), &out) == nil {
			return out
		}
	}
	return map[string]any{}
}

func mapValues(value any) []any {
	switch item := value.(type) {
	case map[string]any:
		out := make([]any, 0, len(item))
		for _, child := range item {
			out = append(out, child)
		}
		return out
	case []any:
		return item
	}
	return nil
}

func planVideo(prompt, ratio, duration, model string) string {
	modelName := videoModelLabel(model)
	if canonicalVideoModel(model) == videoModelID {
		modelName = "根据账号可用能力自动选择"
	}
	if strings.TrimSpace(duration) == "" {
		duration = "由模型决定"
	}
	return fmt.Sprintf(
		"视频生成方案\n- 模型：%s\n- 比例：%s\n- 时长：%s\n- 画面：%s\n\n回复“确认生成”后，我会直接提交一次视频任务，不再重复询问参数。",
		modelName, ratio, duration, prompt,
	)
}

func resultText(result doubaoVideoResult) string {
	if len(result.Videos) == 0 {
		if strings.TrimSpace(result.Text) != "" {
			return result.Text
		}
		return "豆包没有返回可下载的视频地址。"
	}
	var builder strings.Builder
	builder.WriteString("视频生成完成。\n\n")
	for index, video := range result.Videos {
		if len(result.Videos) > 1 {
			builder.WriteString(fmt.Sprintf("%d. ", index+1))
		}
		builder.WriteString("视频: " + video.URL + "\n")
		if video.CoverURL != "" {
			builder.WriteString("封面: " + video.CoverURL + "\n")
		}
		if video.Width > 0 && video.Height > 0 {
			builder.WriteString(fmt.Sprintf("尺寸: %dx%d\n", video.Width, video.Height))
		}
	}
	if result.TaskID != "" {
		builder.WriteString("\n任务 ID: " + result.TaskID)
	}
	if result.Vid != "" {
		builder.WriteString("\n视频 ID: " + result.Vid)
	}
	if result.Remain > 0 {
		builder.WriteString(fmt.Sprintf("\n账号剩余视频次数: %d", result.Remain))
	}
	return strings.TrimSpace(builder.String())
}

func runVideo(req pluginapi.ExecutorRequest, sender httpSender) (string, doubaoVideoResult, error) {
	c, err := readCredential(req.StorageJSON)
	if err != nil {
		return "", doubaoVideoResult{}, pluginabi.NewError("invalid_auth", "invalid Doubao credential", http.StatusUnauthorized)
	}
	format := formatOf(req)
	prompt, confirmed, err := extractVideoRequest(format, req.Payload)
	if err != nil {
		return "", doubaoVideoResult{}, pluginabi.NewError("invalid_request", err.Error(), http.StatusBadRequest)
	}
	ratio, err := videoRatioFromPayload(req.Payload)
	if err != nil {
		return "", doubaoVideoResult{}, pluginabi.NewError("invalid_request", err.Error(), http.StatusBadRequest)
	}
	duration := videoDurationFromPayload(prompt, req.Payload)
	model := canonicalVideoModel(req.Model)
	if !confirmed {
		result := doubaoVideoResult{Plan: true, Model: model, Text: planVideo(prompt, ratio, duration, model)}
		return result.Text, result, nil
	}
	pack, _ := fetchVideoSkillPack(c, sender)
	selection, err := resolveVideoModel(pack, model)
	if err != nil {
		return "", doubaoVideoResult{}, pluginabi.NewError("invalid_request", err.Error(), http.StatusBadRequest)
	}
	resolution := videoResolutionFromPayload(req.Payload)
	modelLabel := videoModelLabel(selection.ShowName)
	if strings.TrimSpace(modelLabel) == "" {
		modelLabel = videoModelLabel(model)
	}
	result, err := generateDoubaoAgentVideo(c, prompt, ratio, duration, selection.Value, modelLabel, resolution, sender)
	if err != nil {
		return "", doubaoVideoResult{}, err
	}
	result.Model = model
	result.ModelValue = selection.Value
	if remain, _, infoErr := fetchVideoGenInfo(c, sender); infoErr == nil {
		result.Remain = remain
	}
	return resultText(result), result, nil
}

func executeVideo(req pluginapi.ExecutorRequest, sender httpSender) (pluginapi.ExecutorResponse, error) {
	release := acquire(req.AuthID)
	defer release()
	text, result, err := runVideo(req, sender)
	if err != nil {
		return pluginapi.ExecutorResponse{}, err
	}
	return pluginapi.ExecutorResponse{
		Payload: encodeMessage(formatOf(req), canonicalVideoModel(req.Model), text, ""),
		Headers: http.Header{"Content-Type": {"application/json"}},
		Metadata: map[string]any{
			"upstream_host":  "www.doubao.com",
			"upstream_model": canonicalVideoModel(req.Model),
			"agent_mode":     true,
			"plan":           result.Plan,
			"task_id":        result.TaskID,
			"video_count":    len(result.Videos),
		},
	}, nil
}

func executeVideoStream(req pluginapi.ExecutorRequest, callbackID, streamID string, sender httpSender) (map[string]any, error) {
	if streamID == "" {
		return nil, pluginabi.NewError("invalid_request", "stream ID required", http.StatusBadRequest)
	}
	release := acquire(req.AuthID)
	stream := newStreamEncoder(formatOf(req), canonicalVideoModel(req.Model), dataFramedChunks(req))
	go func() {
		defer release()
		emit := func(frames [][]byte) bool {
			for _, frame := range frames {
				if len(frame) > 0 && emitStreamChunk(streamID, frame) != nil {
					return false
				}
			}
			return true
		}
		if !emit(stream.Start()) {
			closeStream(streamID, "downstream closed")
			return
		}
		text, _, err := runVideo(req, sender)
		if err != nil {
			closeStream(streamID, err.Error())
			return
		}
		if !emit(stream.Handle(doubaoMessage{Text: text})) {
			closeStream(streamID, "downstream closed")
			return
		}
		if !emit(stream.Finish()) {
			closeStream(streamID, "downstream closed")
			return
		}
		closeStream(streamID, "")
	}()
	return map[string]any{"headers": http.Header{"Content-Type": {"text/event-stream"}, "Cache-Control": {"no-cache"}}}, nil
}
