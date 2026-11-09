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

var doubaoModels = []modelSpec{
	{ID: "doubao", Display: "豆姐 Auto", Context: 256000, Output: 32000},
	{ID: "doubao-seed-2.0", Display: "豆姐 Seed 2.0", Context: 256000, Output: 32000},
	{ID: "doubao-seed-2.0-lite", Display: "豆姐 Seed 2.0 Lite", Context: 256000, Output: 32000},
	{ID: "doubao-seed-2.0-code", Display: "豆姐 Seed 2.0 Code", Context: 256000, Output: 32000},
	{ID: "doubao-seed-1.6", Display: "豆姐 Seed 1.6", Context: 256000, Output: 32000},
	{ID: "doubao-seed-1.6-thinking", Display: "豆姐 Seed 1.6 思考", Context: 256000, Output: 32000},
	{ID: "doubao-seed-1.6-flash", Display: "豆姐 Seed 1.6 Flash", Context: 256000, Output: 32000},
	{ID: "doubao-pro", Display: "豆姐 Pro", Context: 131072, Output: 16384},
	{
		ID:          "doubao-video",
		Display:     "豆姐视频（自动）",
		Description: "Generate a video through the signed-in Doubao agent; the agent selects the available model.",
		Context:     8192,
		Output:      2048,
		Version:     "auto",
		Outputs:     []string{"text", "video"},
	},
	{ID: "seedance-2.5", Display: "Seedance 2.5", Description: "Use Seedance 2.5 through the Doubao video agent.", Context: 8192, Output: 2048, Version: "2.5", Outputs: []string{"text", "video"}},
	{ID: "seedance-2.0", Display: "Seedance 2.0", Description: "Use Seedance 2.0 through the Doubao video agent.", Context: 8192, Output: 2048, Version: "2.0", Outputs: []string{"text", "video"}},
	{ID: "seedance-2.0-fast", Display: "Seedance 2.0 Fast", Description: "Use Seedance 2.0 Fast through the Doubao video agent.", Context: 8192, Output: 2048, Version: "2.0-fast", Outputs: []string{"text", "video"}},
	{ID: "seedance-2.0-mini", Display: "Seedance 2.0 Mini", Description: "Use Seedance 2.0 Mini through the Doubao video agent.", Context: 8192, Output: 2048, Version: "2.0-mini", Outputs: []string{"text", "video"}},
}

func staticModels() []pluginapi.ModelInfo {
	out := make([]pluginapi.ModelInfo, 0, len(doubaoModels))
	for _, spec := range doubaoModels {
		out = append(out, pluginapi.ModelInfo{
			ID:                         spec.ID,
			Object:                     "model",
			OwnedBy:                    credentialProvider,
			Type:                       credentialProvider,
			Name:                       spec.ID,
			DisplayName:                spec.Display,
			ContextLength:              spec.Context,
			InputTokenLimit:            spec.Context,
			MaxCompletionTokens:        spec.Output,
			OutputTokenLimit:           spec.Output,
			Version:                    spec.Version,
			Description:                spec.Description,
			SupportedGenerationMethods: []string{"chat"},
			SupportedInputModalities:   []string{"text"},
			SupportedOutputModalities:  spec.Outputs,
			SupportedParameters:        []string{"prompt", "ratio"},
			Thinking:                   &pluginapi.ThinkingSupport{Levels: []string{"none", "high"}},
		})
	}
	return out
}

func wantsDeepThink(model string) bool {
	model = strings.ToLower(model)
	return strings.Contains(model, "think") || strings.Contains(model, "reason")
}
