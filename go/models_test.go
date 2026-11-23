package main

import (
	"strings"
	"testing"
)

func TestStaticModelsExposeOnlyVideo(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = configure(nil) }()
	models := staticModels()
	if len(models) != len(videoModels) {
		t.Fatalf("static model count = %d, want %d", len(models), len(videoModels))
	}
	for _, model := range models {
		if model.Thinking != nil {
			t.Fatalf("video model %s must not expose thinking levels", model.ID)
		}
		if !strings.HasPrefix(model.ID, "doujie/") {
			t.Fatalf("model %q is not prefixed", model.ID)
		}
		if model.Name != model.ID {
			t.Fatalf("model %q name mismatch: %q", model.ID, model.Name)
		}
		if model.ContextLength <= 0 || model.OutputTokenLimit <= 0 || model.MaxCompletionTokens <= 0 {
			t.Fatalf("video model %s has invalid limits: %#v", model.ID, model)
		}
	}
}

func TestModelPrefixConfig(t *testing.T) {
	cases := []struct {
		raw    string
		prefix string
		id     string
	}{
		{"", "doujie/", "doujie/doubao-video"},
		{"model_prefix: acp/", "acp/", "acp/doubao-video"},
		{"model_prefix: none", "", "doubao-video"},
		{"model_prefix: \"-\"", "", "doubao-video"},
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

func TestGlobalCatalogExposesDefaultModel(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = configure(nil) }()
	models, err := modelsForCredential(credential{Type: credentialProvider, Site: siteGlobal}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "doujie/doubao" {
		t.Fatalf("global catalog = %#v", models)
	}
}

func TestDomesticCatalogRequiresTransport(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = configure(nil) }()
	if _, err := modelsForCredential(credential{Type: credentialProvider, Site: siteCN}, nil); err == nil {
		t.Fatal("domestic catalog without a transport must fail instead of falling back")
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
