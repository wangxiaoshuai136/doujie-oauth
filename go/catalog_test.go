package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const catalogFixture = `{
  "code": 0,
  "data": {
    "entry_list": [
      {
        "action_type": 11,
        "action_bar_key": "coco_deep_thinking",
        "active_switch_conf": {
          "switch_type": 1,
          "use_menu_conf_v2": true,
          "menu_conf_v2": {
            "use_mode_model_list": true,
            "mode_list": {
              "item_list": [
                {
                  "mode_id": "1",
                  "name": "对话",
                  "agent_mode": 2,
                  "show_reasoning_effort": false,
                  "model_list": {
                    "item_list": [
                      {
                        "model_item_key": "0",
                        "item_id": 0,
                        "name": "快速",
                        "default_mode": "1",
                        "conversation_mode_list": ["chat", "creation"],
                        "reasoning_effort_config": {
                          "default_level": 3,
                          "option_list": [
                            {"level": 3, "name": "低"},
                            {"level": 4, "name": "中"},
                            {"level": 5, "name": "高"}
                          ]
                        }
                      },
                      {
                        "model_item_key": "3",
                        "item_id": 3,
                        "name": "专家",
                        "default_mode": "1",
                        "conversation_mode_list": ["chat", "creation"],
                        "reasoning_effort_config": {
                          "default_level": 4,
                          "option_list": [
                            {"level": 3, "name": "低"},
                            {"level": 4, "name": "中"},
                            {"level": 5, "name": "高"}
                          ]
                        }
                      }
                    ]
                  }
                },
                {
                  "mode_id": "3",
                  "name": "工作任务",
                  "agent_mode": 1,
                  "show_reasoning_effort": true,
                  "model_list": {
                    "item_list": [
                      {
                        "model_item_key": "9",
                        "item_id": 9,
                        "name": "自动",
                        "default_mode": "3",
                        "model_extra_params": {"total_window_size": "256000"},
                        "conversation_mode_list": ["office", "creation"],
                        "reasoning_effort_config": {
                          "default_level": 4,
                          "option_list": [
                            {"level": 3, "name": "低"},
                            {"level": 4, "name": "中"},
                            {"level": 5, "name": "高"}
                          ]
                        }
                      },
                      {
                        "model_item_key": "5",
                        "item_id": 5,
                        "name": "豆包 2.1 Pro",
                        "default_mode": "3",
                        "model_extra_params": {"total_window_size": "256000"},
                        "conversation_mode_list": ["office", "creation"],
                        "reasoning_effort_config": {
                          "default_level": 5,
                          "option_list": [
                            {"level": 3, "name": "低"},
                            {"level": 4, "name": "中"},
                            {"level": 5, "name": "高"}
                          ]
                        }
                      },
                      {
                        "model_item_key": "seed-lite-7b",
                        "item_id": 10001,
                        "name": "豆包 2.1 Lite",
                        "default_mode": "3",
                        "model_extra_params": {"memory_profile": "256k", "total_window_size": "256000"},
                        "conversation_mode_list": ["office", "creation"],
                        "reasoning_effort_config": {
                          "default_level": 4,
                          "option_list": [
                            {"level": 3, "name": "低"},
                            {"level": 4, "name": "中"},
                            {"level": 5, "name": "高"}
                          ]
                        }
                      },
                      {
                        "model_item_key": "person-qwen4-max",
                        "item_id": 10001,
                        "name": "Qwen 4 Max",
                        "default_mode": "3",
                        "model_extra_params": {"memory_profile": "300k_230", "total_window_size": "300000"},
                        "conversation_mode_list": ["office", "creation"],
                        "reasoning_effort_config": {
                          "default_level": 5,
                          "option_list": [
                            {"level": 3, "name": "低"},
                            {"level": 5, "name": "高"},
                            {"level": 6, "name": "极高"}
                          ]
                        }
                      },
                      {
                        "model_item_key": "person-new-261231",
                        "item_id": 10002,
                        "name": "New Paid Model",
                        "default_mode": "3",
                        "subscribe_config": {"need_upgrade": true},
                        "reasoning_effort_config": {
                          "default_level": 5,
                          "option_list": [{"level": 5, "name": "高"}]
                        }
                      }
                    ]
                  }
                }
              ]
            }
          }
        }
      }
    ]
  }
}`

type catalogTestSender struct {
	body     []byte
	status   int
	err      error
	requests []pluginapi.HTTPRequest
}

func testCatalogSender() httpSender {
	return &catalogTestSender{body: []byte(catalogFixture)}
}

func (s *catalogTestSender) Do(req pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
	s.requests = append(s.requests, req)
	status := s.status
	if status == 0 {
		status = http.StatusOK
	}
	return pluginapi.HTTPResponse{StatusCode: status, Body: s.body}, s.err
}

func resetCatalogCacheForTest() {
	catalogCacheMu.Lock()
	defer catalogCacheMu.Unlock()
	catalogCache = map[string]catalogCacheEntry{}
}

func primeCatalogForTest(t *testing.T, c credential) {
	t.Helper()
	specs, err := parseCatalogResponse([]byte(catalogFixture))
	if err != nil {
		t.Fatal(err)
	}
	storeCatalogCache(catalogCacheKey(c), specs, nil, time.Hour)
}

func catalogTestCredential() credential {
	return credential{
		Type:     credentialProvider,
		Site:     siteCN,
		Cookies:  map[string]string{"sessionid": "test"},
		DeviceID: "7111111111111111",
		WebID:    "7222222222222222222",
		FP:       "verify_test_test_test_test_test",
	}
}

func TestParseCatalogResponseBuildsStableModels(t *testing.T) {
	specs, err := parseCatalogResponse([]byte(catalogFixture))
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]catalogSpec{}
	for _, spec := range specs {
		byID[spec.ID] = spec
	}
	if len(specs) != 6 {
		t.Fatalf("catalog size = %d, want 6 (upgrade-only model must be hidden): %#v", len(specs), byID)
	}
	if _, exists := byID["new"]; exists {
		t.Fatal("need_upgrade model leaked into the catalog")
	}
	fast, ok := byID["doubao-seed-1.6-flash"]
	if !ok || fast.Selection.ModeID != "1" || fast.Selection.AgentMode != 2 || fast.Selection.ConversationMode != "1" {
		t.Fatalf("fast model = %#v", fast)
	}
	auto, ok := byID["doubao"]
	if !ok {
		t.Fatalf("auto model missing: %#v", byID)
	}
	if auto.Selection.ItemKey != "9" || auto.Selection.ModeID != "3" || auto.Selection.AgentMode != 1 {
		t.Fatalf("auto selection = %#v", auto.Selection)
	}
	if auto.Context != 256000 || auto.DefaultEffort != 4 {
		t.Fatalf("auto context/effort = %d/%d", auto.Context, auto.DefaultEffort)
	}
	qwen, ok := byID["qwen4-max"]
	if !ok {
		t.Fatalf("qwen4-max missing: %#v", byID)
	}
	if qwen.Context != 300000 || qwen.Selection.ItemKey != "person-qwen4-max" || qwen.Selection.ItemID != "10001" {
		t.Fatalf("qwen selection = %#v", qwen)
	}
	if len(qwen.Efforts) != 3 || qwen.Efforts[0] != 3 || qwen.Efforts[2] != 6 || qwen.DefaultEffort != 5 {
		t.Fatalf("qwen efforts = %#v default=%d", qwen.Efforts, qwen.DefaultEffort)
	}
}

func TestStableCatalogModelIDs(t *testing.T) {
	cases := map[string]string{
		"9":                           "doubao",
		"person-kimi-k3-1-pro-261111": "kimi-k3.1-pro",
		"person-qwen3-8-max-261201":   "qwen3-8-max",
		"person-foo-bar-261111":       "foo-bar",
		"person-glm-5-3-flash-261111": "glm-5.3-flash",
	}
	for key, want := range cases {
		if got := stableCatalogModelID(key, "10001", ""); got != want {
			t.Fatalf("stable ID for %q = %q, want %q", key, got, want)
		}
	}
}

func TestCatalogEffortSelection(t *testing.T) {
	specs, err := parseCatalogResponse([]byte(catalogFixture))
	if err != nil {
		t.Fatal(err)
	}
	qwen := specs[len(specs)-1]
	if qwen.ID != "qwen4-max" {
		t.Fatalf("last spec = %q", qwen.ID)
	}
	if got := qwen.Selection.effectiveEffort(""); got != 5 {
		t.Fatalf("default effort = %d", got)
	}
	if got := qwen.Selection.effectiveEffort("low"); got != 3 {
		t.Fatalf("low effort = %d", got)
	}
	if got := qwen.Selection.effectiveEffort("xhigh"); got != 6 {
		t.Fatalf("xhigh effort = %d", got)
	}
	if got := qwen.Selection.effectiveEffort("medium"); got != 5 {
		t.Fatalf("unsupported medium should fall back to default, got %d", got)
	}
	if got := qwen.Selection.effectiveEffort("max"); got != 5 {
		t.Fatalf("unsupported max should fall back to default, got %d", got)
	}
}

func TestModelsForCredentialUsesCatalog(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = configure(nil) }()
	resetCatalogCacheForTest()
	defer resetCatalogCacheForTest()

	sender := &catalogTestSender{body: []byte(catalogFixture)}
	c := catalogTestCredential()
	models, err := modelsForCredential(c, sender)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]pluginapi.ModelInfo{}
	for _, model := range models {
		byID[cleanModelID(model.ID)] = model
	}
	if _, ok := byID["qwen4-max"]; !ok {
		t.Fatalf("dynamic model missing: %#v", byID)
	}
	if _, ok := byID["doubao-video"]; !ok {
		t.Fatal("video model should remain in the catalog")
	}
	if len(sender.requests) != 1 || !strings.Contains(sender.requests[0].URL, "/alice/slot/action_bar_v3/brief_list?") {
		t.Fatalf("unexpected catalog request: %#v", sender.requests)
	}
	if got := sender.requests[0].Headers.Get("Cookie"); !strings.Contains(got, "sessionid=test") {
		t.Fatalf("catalog request did not carry cookies: %q", got)
	}
	selection, ok := selectionForModelForCredential(c, "qwen4-max")
	if !ok || selection.ItemKey != "person-qwen4-max" || selection.ModeID != "3" {
		t.Fatalf("dynamic selection = %#v ok=%v", selection, ok)
	}
	if _, err := modelsForCredential(c, sender); err != nil {
		t.Fatal(err)
	}
	if len(sender.requests) != 1 {
		t.Fatalf("catalog cache missed: %d requests", len(sender.requests))
	}
}

func TestCatalogResponseRejectsLegacyMenu(t *testing.T) {
	legacy := []byte(`{"code":0,"data":{"entry_list":[{"action_type":11,"action_bar_key":"coco_deep_thinking","active_switch_conf":{"switch_type":1,"use_menu_conf_v2":false}}]}}`)
	if _, err := parseCatalogResponse(legacy); err == nil {
		t.Fatal("legacy menu must not be treated as a valid catalog")
	}
}

func TestBuildRequestUsesDynamicSelectionAndEffort(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = configure(nil) }()
	resetCatalogCacheForTest()
	defer resetCatalogCacheForTest()

	sender := &catalogTestSender{body: []byte(catalogFixture)}
	c := catalogTestCredential()
	storage, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"model":"qwen4-max","reasoning_effort":"xhigh","messages":[{"role":"user","content":"hello"}]}`)
	plan, err := buildRequest(pluginapi.ExecutorRequest{Payload: payload, Model: "qwen4-max", Format: "openai", StorageJSON: storage}, sender)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Option struct {
			ModelConfig map[string]any `json:"model_config"`
			Aggregate   map[string]any `json:"aggregate_params"`
			NeedDeep    int            `json:"need_deep_think"`
		} `json:"option"`
		Ext map[string]any `json:"ext"`
	}
	if err := json.Unmarshal(plan.HTTP.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Option.ModelConfig["model_item_key"] != "person-qwen4-max" || body.Option.ModelConfig["reasoning_effort"] != float64(6) {
		t.Fatalf("dynamic model config = %#v", body.Option.ModelConfig)
	}
	if body.Option.Aggregate["mode_id"] != "3" || body.Option.Aggregate["agent_mode"] != "1" || body.Option.Aggregate["conversation_mode"] != "2" {
		t.Fatalf("dynamic aggregate = %#v", body.Option.Aggregate)
	}
	if body.Ext["use_deep_think"] != "10001" || body.Option.NeedDeep != 10001 {
		t.Fatalf("dynamic deep-think fields = %#v / %d", body.Ext["use_deep_think"], body.Option.NeedDeep)
	}
	if len(sender.requests) > 1 {
		t.Fatalf("catalog was fetched more than once: %d", len(sender.requests))
	}
}

func TestBuildRequestModelSuffixEffort(t *testing.T) {
	if err := configure(nil); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = configure(nil) }()
	c := catalogTestCredential()
	resetCatalogCacheForTest()
	defer resetCatalogCacheForTest()
	primeCatalogForTest(t, c)
	storage, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		model string
		want  float64
	}{
		{"doubao-pro(low)", 3},
		{"doubao-pro(high)", 5},
		{"doubao-pro(medium)", 4},
	} {
		payload := []byte(`{"model":"` + tc.model + `","messages":[{"role":"user","content":"hello"}]}`)
		plan, err := buildRequest(pluginapi.ExecutorRequest{Payload: payload, Model: tc.model, Format: "openai", StorageJSON: storage}, nil)
		if err != nil {
			t.Fatalf("model %q: %v", tc.model, err)
		}
		var body struct {
			Option struct {
				ModelConfig map[string]any `json:"model_config"`
			} `json:"option"`
		}
		if err := json.Unmarshal(plan.HTTP.Body, &body); err != nil {
			t.Fatal(err)
		}
		if got := body.Option.ModelConfig["reasoning_effort"]; got != tc.want {
			t.Fatalf("model %q effort = %#v, want %v", tc.model, got, tc.want)
		}
	}
}
