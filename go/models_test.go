package main

import "testing"

func TestStaticModelsThinkingLevels(t *testing.T) {
	models := staticModels()
	byID := map[string]struct {
		levels   []string
		zero     bool
		dynamic  bool
		thinking bool
	}{}
	for _, model := range models {
		entry := struct {
			levels   []string
			zero     bool
			dynamic  bool
			thinking bool
		}{thinking: model.Thinking != nil}
		if model.Thinking != nil {
			entry.levels = model.Thinking.Levels
			entry.zero = model.Thinking.ZeroAllowed
			entry.dynamic = model.Thinking.DynamicAllowed
		}
		byID[model.ID] = entry
	}

	text := byID["doubao"]
	if !text.thinking || !text.zero || !text.dynamic || len(text.levels) != 3 || text.levels[0] != "low" || text.levels[2] != "high" {
		t.Fatalf("unexpected text thinking support: %+v", text)
	}
	video := byID["doubao-video"]
	if video.thinking {
		t.Fatalf("video model must not expose thinking levels: %+v", video)
	}
}
