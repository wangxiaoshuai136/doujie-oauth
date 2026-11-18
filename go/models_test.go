package main

import (
	"strings"
	"testing"
)

func TestStaticModelsThinkingLevels(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = configure(nil) }()
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

	text := byID["doujie/doubao"]
	if !text.thinking || !text.zero || !text.dynamic || len(text.levels) != 3 || text.levels[0] != "low" || text.levels[2] != "high" {
		t.Fatalf("unexpected text thinking support: %+v", text)
	}
	video := byID["doujie/doubao-video"]
	if video.thinking {
		t.Fatalf("video model must not expose thinking levels: %+v", video)
	}
}

func TestModelsForCredentialFiltersGlobalCatalog(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = configure(nil) }()
	cn := modelsForCredential(credential{Type: credentialProvider, Site: siteCN})
	if len(cn) < 10 {
		t.Fatalf("domestic catalog unexpectedly short: %d", len(cn))
	}
	global := modelsForCredential(credential{Type: credentialProvider, Site: siteGlobal})
	if len(global) != 1 || global[0].ID != "doujie/doubao" {
		t.Fatalf("global catalog should expose only the domestic-compatible text model: %+v", global)
	}
}

func TestModelTokenLimitsFitCombinedContext(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = configure(nil) }()
	for _, model := range staticModels() {
		if model.InputTokenLimit <= 0 || model.OutputTokenLimit <= 0 || model.ContextLength <= 0 {
			t.Fatalf("model %s has invalid token limits: input=%d output=%d context=%d", model.ID, model.InputTokenLimit, model.OutputTokenLimit, model.ContextLength)
		}
		if model.InputTokenLimit+model.OutputTokenLimit != model.ContextLength {
			t.Fatalf("model %s input+output=%d exceeds combined context=%d", model.ID, model.InputTokenLimit+model.OutputTokenLimit, model.ContextLength)
		}
		if model.MaxCompletionTokens != model.OutputTokenLimit {
			t.Fatalf("model %s completion limit mismatch: %d vs %d", model.ID, model.MaxCompletionTokens, model.OutputTokenLimit)
		}
	}
}

func TestModelPrefixCatalog(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = configure(nil) }()
	if configuredModelPrefix() != "doujie/" {
		t.Fatalf("unexpected default prefix: %q", configuredModelPrefix())
	}
	for _, model := range staticModels() {
		if !strings.HasPrefix(model.ID, "doujie/") {
			t.Fatalf("model %q is not prefixed", model.ID)
		}
		if model.Name != model.ID {
			t.Fatalf("model %q name mismatch: %q", model.ID, model.Name)
		}
	}
}

func TestModelPrefixConfig(t *testing.T) {
	cases := []struct {
		raw    string
		prefix string
		id     string
	}{
		{"", "doujie/", "doujie/doubao"},
		{"model_prefix: acp/", "acp/", "acp/doubao"},
		{"model_prefix: none", "", "doubao"},
		{"model_prefix: \"-\"", "", "doubao"},
	}
	for _, tc := range cases {
		if err := configure([]byte(tc.raw)); err != nil {
			t.Fatalf("configure %q: %v", tc.raw, err)
		}
		if got := configuredModelPrefix(); got != tc.prefix {
			t.Fatalf("prefix %q: got %q want %q", tc.raw, got, tc.prefix)
		}
		models := staticModels()
		if len(models) == 0 || models[0].ID != tc.id {
			t.Fatalf("prefix %q: first model = %#v", tc.raw, models)
		}
	}
	_ = configure(nil)
}

func TestSelectionForModelAcceptsPrefixAndLegacyAliases(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = configure(nil) }()
	for _, model := range []string{"glm-5.3-flash", "doujie/glm-5.3-flash", "doubao/glm-5.3-flash", "doujie/glm-5.3-flash(high)"} {
		selection, ok := selectionForModel(model)
		if !ok || selection.ItemKey != "person-glm-5-3-flash-261111" {
			t.Fatalf("model %q: %#v ok=%v", model, selection, ok)
		}
	}
}

func TestVideoModelAcceptsPrefix(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = configure(nil) }()
	if !isVideoModel("doujie/doubao-video") || !isVideoModel("doujie/seedance-2.0-fast") {
		t.Fatal("prefixed video models were not recognized")
	}
	if got := canonicalVideoModel("doujie/doubao-video"); got != videoModelID {
		t.Fatalf("canonical video model = %q", got)
	}
}
func TestVideoResponseModelEchoesRequest(t *testing.T) {
	if got := videoResponseModel("doujie/seedance-2.0"); got != "doujie/seedance-2.0" {
		t.Fatalf("prefixed video response model = %q", got)
	}
	if got := videoResponseModel("  "); got != videoModelID {
		t.Fatalf("empty video response model = %q", got)
	}
}
