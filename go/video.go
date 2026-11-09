package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const (
	videoModelID      = "doubao-video"
	videoModelVersion = "seedance_2.5"
	videoContentType  = 2020
	videoResultType   = 2021
	videoSkillType    = 17
	defaultVideoRatio = "16:9"
)

var videoModelAliases = map[string]struct{}{
	"doubao-video":         {},
	"doubao-session-video": {},
	"doubao-seedance-2.5":  {},
	"doubao-seedance-2_5":  {},
	"seedance-2.5":         {},
	"seedance-2_5":         {},
	"seedance_2.5":         {},
}

type generatedVideo struct {
	URL      string
	CoverURL string
	Width    int
	Height   int
	Duration float64
}

type doubaoVideoResult struct {
	TaskID string
	Videos []generatedVideo
	Text   string
}

func isVideoModel(model string) bool {
	_, ok := videoModelAliases[strings.ToLower(strings.TrimSpace(model))]
	return ok
}

func canonicalVideoModel(model string) string {
	if isVideoModel(model) {
		return videoModelID
	}
	return strings.TrimSpace(model)
}

func videoPayload(prompt, ratio, modelVersion string) map[string]any {
	content := map[string]any{"text": prompt}
	if ratio != "" {
		content["ratio"] = ratio
	}
	if modelVersion != "" {
		content["model_version"] = modelVersion
	}
	message := map[string]any{
		"content":      string(mustJSON(content)),
		"content_type": videoContentType,
		"attachments":  []any{},
		"references":   []any{},
		"skill": map[string]any{
			"skill_type":            videoSkillType,
			"skill_type_no_default": videoSkillType,
			"skill_id":              fmt.Sprintf("%d", videoSkillType),
			"skill_id_no_default":   fmt.Sprintf("%d", videoSkillType),
		},
	}
	return map[string]any{
		"messages": []any{message},
		"completion_option": map[string]any{
			"is_regen":                 false,
			"with_suggest":             true,
			"need_create_conversation": true,
			"launch_stage":             1,
			"is_replace":               false,
			"is_delete":                false,
			"is_ai_playground":         false,
			"memory_type":              2,
			"message_from":             0,
			"use_deep_think":           false,
			"use_auto_cot":             false,
			"resend_for_regen":         false,
			"enable_commerce_credit":   false,
			"action_bar_skill_id":      videoSkillType,
		},
		"evaluate_option":       map[string]any{"web_ab_params": ""},
		"local_conversation_id": randomUUID(),
		"local_message_id":      randomUUID(),
	}
}

func extractVideoPrompt(format string, payload []byte) (string, error) {
	var root struct {
		Prompt string `json:"prompt"`
	}
	_ = json.Unmarshal(payload, &root)
	if strings.TrimSpace(root.Prompt) != "" {
		return strings.TrimSpace(root.Prompt), nil
	}

	var prompt string
	switch format {
	case "openai":
		var body openAIChatRequest
		if json.Unmarshal(payload, &body) != nil {
			return "", fmt.Errorf("invalid OpenAI Chat Completions request")
		}
		for index := len(body.Messages) - 1; index >= 0; index-- {
			message := body.Messages[index]
			if !strings.EqualFold(strings.TrimSpace(message.Role), "user") {
				continue
			}
			if text := strings.TrimSpace(flattenBlocks(message.Content)); text != "" {
				prompt = text
				break
			}
		}
	case "openai-response":
		var body responsesRequest
		if json.Unmarshal(payload, &body) != nil {
			return "", fmt.Errorf("invalid OpenAI Responses request")
		}
		if text, ok := rawText(body.Input); ok {
			prompt = strings.TrimSpace(text)
		} else {
			var items []struct {
				Type    string          `json:"type"`
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			}
			if json.Unmarshal(body.Input, &items) == nil {
				for index := len(items) - 1; index >= 0; index-- {
					item := items[index]
					if item.Type != "" && item.Type != "message" {
						continue
					}
					if !strings.EqualFold(strings.TrimSpace(item.Role), "user") {
						continue
					}
					if text := strings.TrimSpace(flattenBlocks(item.Content)); text != "" {
						prompt = text
						break
					}
				}
			}
		}
	default:
		var body anthropicRequest
		if json.Unmarshal(payload, &body) != nil {
			return "", fmt.Errorf("invalid Anthropic Messages request")
		}
		for index := len(body.Messages) - 1; index >= 0; index-- {
			message := body.Messages[index]
			if !strings.EqualFold(strings.TrimSpace(message.Role), "user") {
				continue
			}
			if text := strings.TrimSpace(flattenBlocks(message.Content)); text != "" {
				prompt = text
				break
			}
		}
	}
	if prompt == "" {
		return "", fmt.Errorf("a user message with the video prompt is required")
	}
	if utf8.RuneCountInString(prompt) > maxPromptChars {
		return "", fmt.Errorf("video prompt exceeds the Doubao context limit")
	}
	return prompt, nil
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
	ratio = strings.TrimSpace(strings.ToLower(ratio))
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

func doVideoRequest(c credential, path string, params url.Values, body []byte, sender httpSender) (pluginapi.HTTPResponse, error) {
	headers := http.Header{
		"Accept":          {"text/event-stream"},
		"Content-Type":    {"application/json"},
		"Agw-Js-Conv":     {"str"},
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
	target := c.profile().Root + path + params.Encode()
	resp, err := sender.Do(pluginapi.HTTPRequest{
		Method:  http.MethodPost,
		URL:     target,
		Headers: headers,
		Body:    body,
	})
	if err != nil {
		return resp, pluginabi.NewError("transport_error", "Doubao video transport failed", 503)
	}
	if resp.StatusCode != http.StatusOK {
		return resp, upstreamError(resp.StatusCode, resp.Body)
	}
	return resp, nil
}

func generateDoubaoVideo(c credential, prompt, ratio string, timeout time.Duration, sender httpSender) (doubaoVideoResult, error) {
	params := securityParams(c, videoModelID)
	submit, err := doVideoRequest(c, "/samantha/chat/completion?", params, mustJSON(videoPayload(prompt, ratio, videoModelVersion)), sender)
	if err != nil {
		return doubaoVideoResult{}, err
	}
	taskID, videos, answer, failure := parseDoubaoVideoResponse(submit.Body)
	if failure != nil {
		return doubaoVideoResult{}, failure
	}
	if len(videos) > 0 {
		return doubaoVideoResult{TaskID: taskID, Videos: videos, Text: answer}, nil
	}
	if taskID == "" {
		if strings.TrimSpace(answer) != "" {
			return doubaoVideoResult{}, pluginabi.NewError("upstream_rejected", "Doubao did not start a video task: "+sanitizeUpstream(answer), http.StatusBadGateway)
		}
		return doubaoVideoResult{}, pluginabi.NewError("upstream_error", "Doubao did not return a video task", http.StatusBadGateway)
	}

	deadline := time.Now().Add(timeout)
	lastText := answer
	for {
		pollParams := securityParams(c, videoModelID)
		pollParams.Del("fp")
		pollBody := mustJSON(map[string]any{"task_id": taskID, "event_id": 0})
		poll, pollErr := doVideoRequest(c, "/samantha/chat/async/stream?", pollParams, pollBody, sender)
		if pollErr != nil {
			return doubaoVideoResult{}, pollErr
		}
		_, polled, pollText, pollFailure := parseDoubaoVideoResponse(poll.Body)
		if pollFailure != nil {
			return doubaoVideoResult{}, pollFailure
		}
		if len(polled) > 0 {
			return doubaoVideoResult{TaskID: taskID, Videos: polled, Text: pollText}, nil
		}
		if pollText != "" {
			lastText = pollText
		}
		if time.Now().After(deadline) {
			message := "Doubao video task is still running; task id: " + taskID
			if strings.TrimSpace(lastText) != "" {
				message += "; last message: " + sanitizeUpstream(lastText)
			}
			return doubaoVideoResult{}, pluginabi.NewError("timeout", message, http.StatusGatewayTimeout)
		}
		time.Sleep(3 * time.Second)
	}
}

func parseDoubaoVideoResponse(raw []byte) (taskID string, videos []generatedVideo, text string, failure error) {
	seen := map[string]struct{}{}
	var textParts []string
	blocks := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n\n")
	for _, block := range blocks {
		if strings.Contains(block, "event: gateway-error") {
			payload := sseData(block)
			if payload != "" {
				if err := videoGatewayError([]byte(payload)); err != nil {
					return "", nil, "", err
				}
			}
			continue
		}
		payload := sseData(block)
		if payload == "" {
			continue
		}
		var event map[string]any
		if json.Unmarshal([]byte(payload), &event) != nil {
			continue
		}
		if code := asInt(event["error_code"]); code != 0 {
			return "", nil, "", pluginabi.NewError("upstream_rejected", sanitizeUpstream(fmt.Sprintf("Doubao error %d: %v", code, event["error_msg"])), http.StatusBadGateway)
		}
		eventType := asInt(event["event_type"])
		if eventType == 2005 {
			return "", nil, "", pluginabi.NewError("upstream_rejected", sanitizeUpstream(fmt.Sprintf("Doubao video generation failed: %v", event["event_data"])), http.StatusBadGateway)
		}
		if eventType != 2001 {
			continue
		}
		eventData := mapValue(event["event_data"])
		if reason := mapValue(eventData["fin_reason"]); asInt(reason["reason"]) == 1 {
			if asyncTask := mapValue(reason["async_task"]); strings.TrimSpace(fmt.Sprintf("%v", asyncTask["id"])) != "" {
				taskID = strings.TrimSpace(fmt.Sprintf("%v", asyncTask["id"]))
			}
		}
		message := mapValue(eventData["message"])
		content := mapValue(message["content"])
		switch asInt(message["content_type"]) {
		case 2001:
			if value := strings.TrimSpace(fmt.Sprintf("%v", content["text"])); value != "" && value != "<nil>" {
				textParts = append(textParts, value)
			}
		case 2021:
			for _, video := range videosFromContent(content) {
				if _, exists := seen[video.URL]; exists {
					continue
				}
				seen[video.URL] = struct{}{}
				videos = append(videos, video)
			}
		}
	}
	return taskID, videos, strings.Join(textParts, ""), nil
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

func videosFromContent(content map[string]any) []generatedVideo {
	items, ok := content["data"].([]any)
	if !ok {
		items = []any{content}
	}
	out := make([]generatedVideo, 0, len(items))
	for _, raw := range items {
		item := mapValue(raw)
		videoURL := strings.TrimSpace(fmt.Sprintf("%v", firstAny(item["video_url"], item["url"])))
		if videoURL == "" || videoURL == "<nil>" {
			videoURL = videoURLFromModel(item["video_model"])
		}
		if videoURL == "" {
			continue
		}
		coverURL := strings.TrimSpace(fmt.Sprintf("%v", item["cover_url"]))
		if coverURL == "" || coverURL == "<nil>" {
			cover := mapValue(item["cover"])
			coverURL = strings.TrimSpace(fmt.Sprintf("%v", cover["url"]))
			if coverURL == "" || coverURL == "<nil>" {
				coverURL = strings.TrimSpace(fmt.Sprintf("%v", mapValue(cover["image_ori"])["url"]))
				if coverURL == "<nil>" {
					coverURL = ""
				}
			}
		}
		duration, _ := item["duration"].(float64)
		out = append(out, generatedVideo{
			URL:      videoURL,
			CoverURL: coverURL,
			Width:    asInt(item["width"]),
			Height:   asInt(item["height"]),
			Duration: duration,
		})
	}
	return out
}

func videoURLFromModel(value any) string {
	model := mapValue(value)
	for _, raw := range mapValues(model["video_list"]) {
		info := mapValue(raw)
		mainURL := strings.TrimSpace(fmt.Sprintf("%v", info["main_url"]))
		if mainURL == "" || mainURL == "<nil>" {
			continue
		}
		if strings.HasPrefix(mainURL, "http://") || strings.HasPrefix(mainURL, "https://") {
			return mainURL
		}
		if decoded := strings.TrimSpace(string(decodeBase64Loose(mainURL))); strings.HasPrefix(decoded, "http://") || strings.HasPrefix(decoded, "https://") {
			return decoded
		}
	}
	return ""
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

func firstAny(values ...any) any {
	for _, value := range values {
		if value != nil && fmt.Sprintf("%v", value) != "" && fmt.Sprintf("%v", value) != "<nil>" {
			return value
		}
	}
	return nil
}

func resultText(result doubaoVideoResult) string {
	if len(result.Videos) == 0 {
		return "Seedance 2.5 已完成，但豆包没有返回可下载的视频地址。"
	}
	var builder strings.Builder
	builder.WriteString("Seedance 2.5 视频生成完成。\n\n")
	for index, video := range result.Videos {
		if len(result.Videos) > 1 {
			builder.WriteString(fmt.Sprintf("%d. ", index+1))
		}
		builder.WriteString("视频: " + video.URL + "\n")
		if video.CoverURL != "" {
			builder.WriteString("封面: " + video.CoverURL + "\n")
		}
		if video.Duration > 0 {
			builder.WriteString(fmt.Sprintf("时长: %.1fs\n", video.Duration))
		}
		if video.Width > 0 && video.Height > 0 {
			builder.WriteString(fmt.Sprintf("尺寸: %dx%d\n", video.Width, video.Height))
		}
	}
	if result.TaskID != "" {
		builder.WriteString("\n任务 ID: " + result.TaskID)
	}
	return strings.TrimSpace(builder.String())
}

func runVideo(req pluginapi.ExecutorRequest, sender httpSender) (string, doubaoVideoResult, error) {
	c, err := readCredential(req.StorageJSON)
	if err != nil {
		return "", doubaoVideoResult{}, pluginabi.NewError("invalid_auth", "invalid Doubao credential", http.StatusUnauthorized)
	}
	format := formatOf(req)
	prompt, err := extractVideoPrompt(format, req.Payload)
	if err != nil {
		return "", doubaoVideoResult{}, pluginabi.NewError("invalid_request", err.Error(), http.StatusBadRequest)
	}
	ratio, err := videoRatioFromPayload(req.Payload)
	if err != nil {
		return "", doubaoVideoResult{}, pluginabi.NewError("invalid_request", err.Error(), http.StatusBadRequest)
	}
	timeout := time.Duration(getConfig().VideoTimeoutSecond) * time.Second
	if timeout <= 0 {
		timeout = 300 * time.Second
	}
	result, err := generateDoubaoVideo(c, prompt, ratio, timeout, sender)
	if err != nil {
		return "", doubaoVideoResult{}, err
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
		Payload: encodeMessage(formatOf(req), videoModelID, text, ""),
		Headers: http.Header{"Content-Type": {"application/json"}},
		Metadata: map[string]any{
			"upstream_host":  "www.doubao.com",
			"upstream_model": videoModelID,
			"model_version":  videoModelVersion,
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
	stream := newStreamEncoder(formatOf(req), videoModelID, dataFramedChunks(req))
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
