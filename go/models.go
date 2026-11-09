package main

import (
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type modelSpec struct {
	ID      string
	Display string
	Context int64
	Output  int64
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
			SupportedGenerationMethods: []string{"chat"},
			Thinking:                   &pluginapi.ThinkingSupport{Levels: []string{"none", "high"}},
		})
	}
	return out
}

func wantsDeepThink(model string) bool {
	model = strings.ToLower(model)
	return strings.Contains(model, "think") || strings.Contains(model, "reason")
}
