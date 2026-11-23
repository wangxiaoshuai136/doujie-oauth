package main

import (
	"fmt"
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
	Context          int64
	Efforts          []int
	DefaultEffort    int
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

var videoModels = []modelSpec{
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

func modelInfoForSpec(spec modelSpec) pluginapi.ModelInfo {
	var thinking *pluginapi.ThinkingSupport
	if len(spec.Outputs) == 0 {
		thinking = thinkingForEfforts(nil, nil)
	}
	inputLimit := spec.Context - spec.Output
	if inputLimit < 1 {
		inputLimit = spec.Context
	}
	return pluginapi.ModelInfo{
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
	}
}

func staticModels() []pluginapi.ModelInfo {
	out := make([]pluginapi.ModelInfo, 0, len(videoModels))
	for _, spec := range videoModels {
		out = append(out, modelInfoForSpec(spec))
	}
	return out
}

var globalDefaultModel = modelSpec{ID: "doubao", Display: "豆姐 Auto", Context: 256000, Version: "dola"}

func modelsForCredential(c credential, sender httpSender) ([]pluginapi.ModelInfo, error) {
	if c.site() == siteGlobal {
		return []pluginapi.ModelInfo{modelInfoForSpec(globalDefaultModel)}, nil
	}

	specs, err := catalogForCredential(c, sender)
	if err != nil {
		return nil, err
	}
	if len(specs) == 0 {
		return nil, fmt.Errorf("model catalog returned no models")
	}
	models := make([]pluginapi.ModelInfo, 0, len(specs)+5)
	seen := map[string]struct{}{}
	for _, spec := range specs {
		model := catalogModelInfo(spec)
		models = append(models, model)
		seen[cleanModelID(model.ID)] = struct{}{}
	}
	for _, spec := range videoModels {
		if _, exists := seen[spec.ID]; exists {
			continue
		}
		models = append(models, modelInfoForSpec(spec))
	}
	return models, nil
}

func wantsDeepThink(model string) bool {
	model = strings.ToLower(cleanModelID(model))
	return strings.Contains(model, "think") || strings.Contains(model, "reason")
}
