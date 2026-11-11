package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type videoTaskState struct {
	Started time.Time `json:"started_at"`
	Phase   string    `json:"phase"`
}

type videoCooldownState struct {
	Until  time.Time `json:"until"`
	Reason string    `json:"reason"`
}

var videoStateMu sync.Mutex
var videoTasks = map[string]videoTaskState{}
var videoCooldowns = map[string]videoCooldownState{}

func videoStateKey(authID string) string {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return "default"
	}
	return authID
}

func acquireVideoTask(authID string) error {
	key := videoStateKey(authID)
	now := time.Now()
	videoStateMu.Lock()
	defer videoStateMu.Unlock()
	for id, cooldown := range videoCooldowns {
		if !cooldown.Until.After(now) {
			delete(videoCooldowns, id)
		}
	}
	if cooldown, ok := videoCooldowns[key]; ok {
		return pluginabi.NewError("quota_exhausted", fmt.Sprintf("视频额度冷却中，预计 %s 恢复：%s", cooldown.Until.Local().Format("2006-01-02 15:04"), cooldown.Reason), http.StatusTooManyRequests)
	}
	if _, busy := videoTasks[key]; busy {
		return pluginabi.NewError("video_busy", "该账号已有正在处理的视频任务", http.StatusConflict)
	}
	videoTasks[key] = videoTaskState{Started: now, Phase: "planning"}
	return nil
}

func setVideoTaskPhase(authID, phase string) {
	key := videoStateKey(authID)
	videoStateMu.Lock()
	defer videoStateMu.Unlock()
	if task, ok := videoTasks[key]; ok {
		task.Phase = strings.TrimSpace(phase)
		videoTasks[key] = task
	}
}

func releaseVideoTask(authID string) {
	key := videoStateKey(authID)
	videoStateMu.Lock()
	defer videoStateMu.Unlock()
	delete(videoTasks, key)
}

func setVideoCooldown(authID string, until time.Time, reason string) {
	if until.IsZero() || !until.After(time.Now()) {
		return
	}
	key := videoStateKey(authID)
	videoStateMu.Lock()
	defer videoStateMu.Unlock()
	videoCooldowns[key] = videoCooldownState{Until: until.UTC(), Reason: sanitizeUpstream(reason)}
}

func clearVideoCooldown(authID string) int {
	videoStateMu.Lock()
	defer videoStateMu.Unlock()
	if strings.TrimSpace(authID) == "" {
		count := len(videoCooldowns)
		videoCooldowns = map[string]videoCooldownState{}
		return count
	}
	key := videoStateKey(authID)
	if _, ok := videoCooldowns[key]; !ok {
		return 0
	}
	delete(videoCooldowns, key)
	return 1
}

func videoStateSnapshot() map[string]any {
	now := time.Now()
	videoStateMu.Lock()
	defer videoStateMu.Unlock()
	tasks := make([]map[string]any, 0, len(videoTasks))
	for authID, task := range videoTasks {
		tasks = append(tasks, map[string]any{
			"auth_id":    authID,
			"phase":      task.Phase,
			"started_at": task.Started.UTC().Format(time.RFC3339),
			"age_sec":    int64(now.Sub(task.Started).Seconds()),
		})
	}
	cooldowns := make([]map[string]any, 0, len(videoCooldowns))
	for authID, cooldown := range videoCooldowns {
		if !cooldown.Until.After(now) {
			continue
		}
		cooldowns = append(cooldowns, map[string]any{
			"auth_id":    authID,
			"until":      cooldown.Until.UTC().Format(time.RFC3339),
			"reason":     cooldown.Reason,
			"remain_sec": int64(cooldown.Until.Sub(now).Seconds()),
		})
	}
	return map[string]any{
		"plugin":    pluginStoreID,
		"tasks":     tasks,
		"cooldowns": cooldowns,
	}
}

func handleManagementRequest(req pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	path := strings.TrimSpace(req.Path)
	if index := strings.IndexByte(path, '?'); index >= 0 {
		path = path[:index]
	}
	if strings.HasPrefix(path, "/doujie-oauth/") {
		path = "/v0/management" + path
	}
	switch path {
	case "/v0/management/doujie-oauth/video/status":
		if !strings.EqualFold(req.Method, http.MethodGet) && req.Method != "" {
			return pluginapi.ManagementResponse{
				StatusCode: http.StatusMethodNotAllowed,
				Headers:    http.Header{"Allow": {http.MethodGet}},
				Body:       []byte(`{"error":"method not allowed"}`),
			}, nil
		}
		body, _ := json.Marshal(videoStateSnapshot())
		return jsonManagementResponse(http.StatusOK, body), nil
	case "/v0/management/doujie-oauth/video/cooldowns/clear":
		if !strings.EqualFold(req.Method, http.MethodPost) {
			return pluginapi.ManagementResponse{
				StatusCode: http.StatusMethodNotAllowed,
				Headers:    http.Header{"Allow": {http.MethodPost}},
				Body:       []byte(`{"error":"method not allowed"}`),
			}, nil
		}
		var input struct {
			AuthID string `json:"auth_id"`
		}
		_ = json.Unmarshal(req.Body, &input)
		cleared := clearVideoCooldown(input.AuthID)
		body, _ := json.Marshal(map[string]any{"cleared": cleared})
		return jsonManagementResponse(http.StatusOK, body), nil
	default:
		return handleManagementResource(req)
	}
}

func jsonManagementResponse(status int, body []byte) pluginapi.ManagementResponse {
	return pluginapi.ManagementResponse{
		StatusCode: status,
		Headers: http.Header{
			"Content-Type":           {"application/json; charset=utf-8"},
			"Cache-Control":          {"no-store"},
			"X-Content-Type-Options": {"nosniff"},
		},
		Body: body,
	}
}
