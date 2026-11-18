package main

import (
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type modelSpec struct {
	ID          string
	Display     string
	Description string
	Context     int64
	Output      int64
	Version     string
	Outputs     []string
}

type modelSelection struct {
	ItemKey          string
	ItemID           string
	ModeID           string
	ConversationMode string
	AgentMode        int
	ReasoningEffort  int
	ExtraParams      map[string]any
}

const defaultModelPrefix = "doujie/"

func normalizeModelPrefix(value string) string {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "", "none", "-", "off":
		return ""
	}
	if !strings.HasSuffix(value, "/") {
		value += "/"
	}
	return value
}

func configuredModelPrefix() string {
	return normalizeModelPrefix(getConfig().ModelPrefix)
}

func prefixedModelID(id string) string {
	id = strings.TrimSpace(id)
	prefix := configuredModelPrefix()
	if prefix == "" {
		return id
	}
	return prefix + id
}

func splitModelAndEffort(model string) (string, string) {
	model = strings.TrimSpace(model)
	open := strings.LastIndex(model, "(")
	if open > 0 && strings.HasSuffix(model, ")") {
		effort := strings.TrimSpace(model[open+1 : len(model)-1])
		if effort != "" {
			return strings.TrimSpace(model[:open]), effort
		}
	}
	return model, ""
}

func cleanModelID(model string) string {
	model, _ = splitModelAndEffort(model)
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	lower := strings.ToLower(model)
	for _, prefix := range []string{configuredModelPrefix(), "doujie/", "doubao/"} {
		prefix = strings.ToLower(prefix)
		if prefix != "" && strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(model[len(prefix):])
		}
	}
	return model
}

// isWorkMode reports whether the selection belongs to the MOA work-task menu
// (mode_id=3, agent_mode=1) rather than the ordinary chat menu.
func (s modelSelection) isWorkMode() bool {
	return s.ModeID == "3" && s.AgentMode == 1
}

var doubaoModels = []modelSpec{
	{ID: "doubao", Display: "豆姐 Auto", Context: 256000, Output: 32000},
	{ID: "doubao-seed-2.0", Display: "豆姐 Seed 2.0", Context: 256000, Output: 32000},
	{ID: "doubao-seed-2.0-lite", Display: "豆姐 Seed 2.0 Lite", Context: 256000, Output: 32000},
	{ID: "doubao-seed-2.0-code", Display: "豆姐 Seed 2.0 Code", Context: 256000, Output: 32000},
	{ID: "doubao-seed-1.6", Display: "豆姐 Seed 1.6", Context: 256000, Output: 32000},
	{ID: "doubao-seed-1.6-thinking", Display: "豆姐 Seed 1.6 思考", Context: 256000, Output: 32000},
	{ID: "doubao-seed-1.6-flash", Display: "豆姐 Seed 1.6 Flash", Context: 256000, Output: 32000},
	{ID: "doubao-pro", Display: "豆姐 Pro", Context: 256000, Output: 32000},
	{ID: "doubao-2.1-lite", Display: "豆姐 2.1 Lite", Context: 256000, Output: 32000},
	{ID: "doubao-2.1-pro", Display: "豆姐 2.1 Pro", Context: 256000, Output: 32000},
	{ID: "deepseek-v4.1-flash", Display: "DeepSeek V4.1 Flash", Context: 300000, Output: 32000},
	{ID: "deepseek-v4-pro", Display: "DeepSeek V4 Pro", Context: 300000, Output: 32000},
	{ID: "kimi-k3.1-pro", Display: "Kimi K3.1 Pro", Context: 300000, Output: 32000},
	{ID: "kimi-k3.1-pro-high-speed", Display: "Kimi K3.1 Pro 高速", Context: 300000, Output: 32000},
	{ID: "kimi-k3", Display: "Kimi K3", Context: 300000, Output: 32000},
	{ID: "glm-5.3", Display: "GLM 5.3", Context: 300000, Output: 32000},
	{ID: "glm-5.3-flash", Display: "GLM 5.3 Flash", Context: 300000, Output: 32000},
	{
		ID:          "doubao-video",
		Display:     "豆姐视频（自动）",
		Description: "Generate a video through the official Doubao video skill; uses the account default model (currently Seedance 2.0 Mini).",
		Context:     8192,
		Output:      2048,
		Version:     "auto",
		Outputs:     []string{"text", "video"},
	},
	{ID: "seedance-2.5", Display: "Seedance 2.5", Description: "Use Seedance 2.5 when the account's official video skill exposes it.", Context: 8192, Output: 2048, Version: "2.5", Outputs: []string{"text", "video"}},
	{ID: "seedance-2.0", Display: "Seedance 2.0", Description: "Official Seedance 2.0; requires an enhanced Doubao subscription. Use Fast or Mini if unavailable.", Context: 8192, Output: 2048, Version: "2.0", Outputs: []string{"text", "video"}},
	{ID: "seedance-2.0-fast", Display: "Seedance 2.0 Fast", Description: "Use Seedance 2.0 Fast through the official Doubao video skill.", Context: 8192, Output: 2048, Version: "2.0-fast", Outputs: []string{"text", "video"}},
	{ID: "seedance-2.0-mini", Display: "Seedance 2.0 Mini", Description: "Use Seedance 2.0 Mini through the official Doubao video skill.", Context: 8192, Output: 2048, Version: "2.0-mini", Outputs: []string{"text", "video"}},
}

func selectionForModel(model string) (modelSelection, bool) {
	switch strings.ToLower(cleanModelID(model)) {
	case "doubao":
		return modelSelection{
			ItemKey:          "9",
			ItemID:           "9",
			ModeID:           "3",
			ConversationMode: "2",
			AgentMode:        1,
			ReasoningEffort:  5,
			ExtraParams:      map[string]any{"total_window_size": "256000"},
		}, true
	case "doubao-pro", "doubao-2.1-pro":
		return modelSelection{
			ItemKey:          "5",
			ItemID:           "5",
			ModeID:           "3",
			ConversationMode: "2",
			AgentMode:        1,
			ReasoningEffort:  5,
			ExtraParams:      map[string]any{"total_window_size": "256000"},
		}, true
	case "doubao-seed-2.0", "doubao-seed-2.0-code":
		return modelSelection{
			ItemKey:          "5",
			ItemID:           "5",
			ModeID:           "3",
			ConversationMode: "2",
			AgentMode:        1,
			ReasoningEffort:  5,
			ExtraParams:      map[string]any{"total_window_size": "256000"},
		}, true
	case "doubao-seed-2.0-lite", "doubao-2.1-lite":
		return modelSelection{
			ItemKey:          "seed-lite-7b",
			ItemID:           "10001",
			ModeID:           "3",
			ConversationMode: "2",
			AgentMode:        1,
			ReasoningEffort:  5,
			ExtraParams:      map[string]any{"memory_profile": "256k", "total_window_size": "256000"},
		}, true
	case "deepseek-v4.1-flash", "deepseek-v4-1-flash", "deepseek-v4-flash", "person-deepseek-v4-1-flash-261017":
		return modelSelection{
			ItemKey:          "person-deepseek-v4-1-flash-261017",
			ItemID:           "10001",
			ModeID:           "3",
			ConversationMode: "2",
			AgentMode:        1,
			ReasoningEffort:  5,
			ExtraParams:      map[string]any{"memory_profile": "300k_230", "total_window_size": "300000"},
		}, true
	case "deepseek-v4-pro", "deepseek-v4.1-pro", "deepseek-v4-1-pro", "deepseek-v4-pro-ga-260813", "person-deepseek-v4-pro-ga-260813":
		return modelSelection{
			ItemKey:          "person-deepseek-v4-pro-ga-260813",
			ItemID:           "10001",
			ModeID:           "3",
			ConversationMode: "2",
			AgentMode:        1,
			ReasoningEffort:  5,
			ExtraParams:      map[string]any{"memory_profile": "300k_230", "total_window_size": "300000"},
		}, true
	case "kimi-k3.1-pro", "kimi-k3-1-pro", "person-kimi-k3-1-pro-261111":
		return modelSelection{
			ItemKey:          "person-kimi-k3-1-pro-261111",
			ItemID:           "10001",
			ModeID:           "3",
			ConversationMode: "2",
			AgentMode:        1,
			ReasoningEffort:  5,
			ExtraParams:      map[string]any{"memory_profile": "300k_230", "total_window_size": "300000"},
		}, true
	case "kimi-k3.1-pro-high-speed", "kimi-k3-1-pro-high-speed", "kimi-k3-1-pro-highspeed", "person-kimi-k3-1-pro-highspeed-261111":
		return modelSelection{
			ItemKey:          "person-kimi-k3-1-pro-highspeed-261111",
			ItemID:           "10001",
			ModeID:           "3",
			ConversationMode: "2",
			AgentMode:        1,
			ReasoningEffort:  5,
			ExtraParams:      map[string]any{"memory_profile": "300k_230", "total_window_size": "300000"},
		}, true
	case "kimi-k3", "person-kimi-k3-261111":
		return modelSelection{
			ItemKey:          "person-kimi-k3-261111",
			ItemID:           "10001",
			ModeID:           "3",
			ConversationMode: "2",
			AgentMode:        1,
			ReasoningEffort:  5,
			ExtraParams:      map[string]any{"memory_profile": "300k_230", "total_window_size": "300000"},
		}, true
	case "glm-5.3", "glm-5-3", "person-glm-5-3-261111":
		return modelSelection{
			ItemKey:          "person-glm-5-3-261111",
			ItemID:           "10001",
			ModeID:           "3",
			ConversationMode: "2",
			AgentMode:        1,
			ReasoningEffort:  5,
			ExtraParams:      map[string]any{"memory_profile": "300k_230", "total_window_size": "300000"},
		}, true
	case "glm-5.3-flash", "glm-5-3-flash", "person-glm-5-3-flash-261111":
		return modelSelection{
			ItemKey:          "person-glm-5-3-flash-261111",
			ItemID:           "10001",
			ModeID:           "3",
			ConversationMode: "2",
			AgentMode:        1,
			ReasoningEffort:  5,
			ExtraParams:      map[string]any{"memory_profile": "300k_230", "total_window_size": "300000"},
		}, true
	case "doubao-seed-1.6", "doubao-seed-1.6-thinking":
		return modelSelection{
			ItemKey:          "3",
			ItemID:           "3",
			ModeID:           "1",
			ConversationMode: "1",
			AgentMode:        2,
			ReasoningEffort:  4,
			ExtraParams:      map[string]any{"total_window_size": "128000"},
		}, true
	case "doubao-seed-1.6-flash":
		return modelSelection{
			ItemKey:          "0",
			ItemID:           "0",
			ModeID:           "1",
			ConversationMode: "1",
			AgentMode:        2,
			ReasoningEffort:  3,
			ExtraParams:      map[string]any{},
		}, true
	default:
		return modelSelection{}, false
	}
}

func staticModels() []pluginapi.ModelInfo {
	out := make([]pluginapi.ModelInfo, 0, len(doubaoModels))
	for _, spec := range doubaoModels {
		var thinking *pluginapi.ThinkingSupport
		if len(spec.Outputs) == 0 {
			thinking = &pluginapi.ThinkingSupport{
				ZeroAllowed:    true,
				DynamicAllowed: true,
				Levels:         []string{"low", "medium", "high"},
			}
		}
		inputLimit := spec.Context - spec.Output
		if inputLimit < 1 {
			inputLimit = spec.Context
		}
		out = append(out, pluginapi.ModelInfo{
			ID:                         prefixedModelID(spec.ID),
			Object:                     "model",
			OwnedBy:                    credentialProvider,
			Type:                       credentialProvider,
			Name:                       prefixedModelID(spec.ID),
			DisplayName:                spec.Display,
			ContextLength:              spec.Context,
			InputTokenLimit:            inputLimit,
			MaxCompletionTokens:        spec.Output,
			OutputTokenLimit:           spec.Output,
			Version:                    spec.Version,
			Description:                spec.Description,
			SupportedGenerationMethods: []string{"chat"},
			SupportedInputModalities:   []string{"text"},
			SupportedOutputModalities:  spec.Outputs,
			SupportedParameters:        []string{"prompt", "ratio", "seconds", "resolution"},
			Thinking:                   thinking,
		})
	}
	return out
}

func modelsForCredential(c credential) []pluginapi.ModelInfo {
	models := staticModels()
	if c.site() != siteGlobal {
		return models
	}
	filtered := make([]pluginapi.ModelInfo, 0, 1)
	for _, model := range models {
		if cleanModelID(model.ID) == "doubao" {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

func wantsDeepThink(model string) bool {
	model = strings.ToLower(cleanModelID(model))
	return strings.Contains(model, "think") || strings.Contains(model, "reason")
}
