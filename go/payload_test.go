package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func testCredential() credential {
	return credential{
		Type:     credentialProvider,
		Site:     siteCN,
		Cookies:  map[string]string{"sessionid": "s"},
		DeviceID: "7000000000000000",
		WebID:    "7600000000000000000",
		FP:       "verify_x",
	}
}

func buildPayloadForTest(t *testing.T, cfg, model string, c credential) (preparedRequest, error) {
	t.Helper()
	if err := configure([]byte(cfg)); err != nil {
		t.Fatalf("configure: %v", err)
	}
	storage, _ := json.Marshal(c)
	payload := []byte(`{"model":"` + model + `","messages":[{"role":"user","content":"hello"}]}`)
	return buildRequest(pluginapi.ExecutorRequest{Payload: payload, Model: model, StorageJSON: storage}, testCatalogSender())
}

func decodeBody(t *testing.T, plan preparedRequest) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(plan.HTTP.Body, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return body
}

func nestedMap(t *testing.T, value any, key string) map[string]any {
	t.Helper()
	obj, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s is not an object: %#v", key, value)
	}
	child, ok := obj[key].(map[string]any)
	if !ok {
		t.Fatalf("%s is not an object: %#v", key, obj[key])
	}
	return child
}

func TestChatModeSelectionUsesDefaultBotAndOmitsTaskParams(t *testing.T) {
	plan, err := buildPayloadForTest(t, "runtime: cloud", "doubao-seed-1.6-flash", testCredential())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.HTTP.URL, "/chat/completion?") {
		t.Fatalf("unexpected endpoint: %s", plan.HTTP.URL)
	}
	body := decodeBody(t, plan)
	meta := nestedMap(t, body, "client_meta")
	if meta["bot_id"] != defaultBotID {
		t.Fatalf("chat mode must use the normal bot, got %#v", meta["bot_id"])
	}
	ext := nestedMap(t, body, "ext")
	if _, ok := ext["general_task_param"]; ok {
		t.Fatalf("chat mode must not send general_task_param: %#v", ext)
	}
	option := nestedMap(t, body, "option")
	if _, ok := option["general_task_param"]; ok {
		t.Fatalf("chat mode must not send option.general_task_param: %#v", option)
	}
	initExt := nestedMap(t, option, "conversation_init_ext")
	if _, ok := initExt["agent_task_param"]; ok {
		t.Fatalf("chat mode must not send agent_task_param: %#v", initExt)
	}
	modelConfig := nestedMap(t, option, "model_config")
	if modelConfig["model_item_key"] != "0" {
		t.Fatalf("unexpected chat model key: %#v", modelConfig)
	}
	aggregate := nestedMap(t, option, "aggregate_params")
	if aggregate["mode_id"] != "1" || aggregate["agent_mode"] != "2" || aggregate["conversation_mode"] != "1" {
		t.Fatalf("unexpected chat aggregate params: %#v", aggregate)
	}
}

func TestCloudWorkTaskOnlyCarriesRuntimeType(t *testing.T) {
	plan, err := buildPayloadForTest(t, "runtime: cloud", "doubao-pro", testCredential())
	if err != nil {
		t.Fatal(err)
	}
	body := decodeBody(t, plan)
	meta := nestedMap(t, body, "client_meta")
	if meta["bot_id"] != modelSelectionBotID {
		t.Fatalf("work mode must use the selection bot, got %#v", meta["bot_id"])
	}
	option := nestedMap(t, body, "option")
	general := nestedMap(t, option, "general_task_param")
	if general["runtime_type"] != float64(1) {
		t.Fatalf("unexpected cloud runtime: %#v", general)
	}
	agent := nestedMap(t, general, "agent_task_param")
	if len(agent) != 1 || agent["runtime_type"] != float64(1) {
		t.Fatalf("cloud agent task must only contain runtime_type=1: %#v", agent)
	}
}

func TestLocalWorkTaskCarriesClientDeviceFields(t *testing.T) {
	cfg := strings.Join([]string{
		"runtime: local",
		`workspace: D:\WenPaul\Documents\豆包REA`,
		"device_name: TEST-PC",
		"sandbox_auth_type: ask_risky",
		`local_app_id: "497858"`,
	}, "\n")
	plan, err := buildPayloadForTest(t, cfg, "doubao-pro", testCredential())
	if err != nil {
		t.Fatal(err)
	}
	body := decodeBody(t, plan)
	option := nestedMap(t, body, "option")
	general := nestedMap(t, option, "general_task_param")
	if general["runtime_type"] != float64(2) {
		t.Fatalf("unexpected local runtime: %#v", general)
	}
	agent := nestedMap(t, general, "agent_task_param")
	want := map[string]any{
		"runtime_type":      float64(2),
		"device_name":       "TEST-PC",
		"folder_name":       "豆包REA",
		"local_app_id":      "497858",
		"local_device_id":   "7000000000000000",
		"workspace":         `D:\WenPaul\Documents\豆包REA`,
		"sandbox_auth_type": float64(1),
	}
	for key, value := range want {
		if agent[key] != value {
			t.Fatalf("agent_task_param[%q] = %#v, want %#v (all=%#v)", key, agent[key], value, agent)
		}
	}
	initExt := nestedMap(t, option, "conversation_init_ext")
	raw, ok := initExt["agent_task_param"].(string)
	if !ok || raw == "" {
		t.Fatalf("conversation_init_ext.agent_task_param missing: %#v", initExt)
	}
	var initAgent map[string]any
	if err := json.Unmarshal([]byte(raw), &initAgent); err != nil {
		t.Fatal(err)
	}
	if initAgent["runtime_type"] != float64(2) || initAgent["local_device_id"] != "7000000000000000" {
		t.Fatalf("unexpected conversation_init_ext agent task: %#v", initAgent)
	}
	clientOption := nestedMap(t, general, "client_option")
	if clientOption["enable_sandbox"] != true || clientOption["os"] != "Windows" {
		t.Fatalf("unexpected client option: %#v", clientOption)
	}
	if clientOption["workspace"] != `D:\WenPaul\Documents\豆包REA` {
		t.Fatalf("unexpected client workspace: %#v", clientOption)
	}
	if clientOption["sandbox_id"] == "" || clientOption["sandbox_env_id"] == "" || clientOption["client_env_id"] == "" {
		t.Fatalf("client option is missing sandbox identifiers: %#v", clientOption)
	}
}

func TestUnknownModelIsRejectedInsteadOfFallingBack(t *testing.T) {
	_, err := buildPayloadForTest(t, "runtime: cloud", "not-a-real-model", testCredential())
	var apiErr *pluginabi.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected plugin error, got %v", err)
	}
	if apiErr.Code != "model_unavailable" || apiErr.HTTPStatus != 400 {
		t.Fatalf("unexpected error: %#v", apiErr)
	}
}

func TestWorkspaceFolderNameHandlesBothSeparators(t *testing.T) {
	cases := map[string]string{
		`D:\WenPaul\Documents\豆包REA`: "豆包REA",
		`C:/Users/demo/project/`:     "project",
		`/home/user/work`:            "work",
		`plain`:                      "plain",
		``:                           "",
	}
	for input, want := range cases {
		if got := workspaceFolderName(input); got != want {
			t.Fatalf("workspaceFolderName(%q) = %q, want %q", input, got, want)
		}
	}
}
