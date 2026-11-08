package main

import (
	"encoding/json"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

type quotaFakeSender struct {
	summary []byte
	balance []byte
	used    []byte
	lastUse []byte
}

func (f *quotaFakeSender) Do(req pluginapi.HTTPRequest) (pluginapi.HTTPResponse, error) {
	switch {
	case contains(req.URL, quotaSummaryPath):
		return pluginapi.HTTPResponse{StatusCode: 200, Body: f.summary}, nil
	case contains(req.URL, cardBalancePath):
		return pluginapi.HTTPResponse{StatusCode: 200, Body: f.balance}, nil
	case contains(req.URL, cardUsePath):
		f.lastUse = req.Body
		return pluginapi.HTTPResponse{StatusCode: 200, Body: f.used}, nil
	default:
		return pluginapi.HTTPResponse{StatusCode: 404, Body: []byte(`{}`)}, nil
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func testCredentialJSON(t *testing.T) []byte {
	t.Helper()
	c := credential{
		Type:     credentialProvider,
		Cookies:  map[string]string{"sessionid": "s", "ttwid": "t"},
		DeviceID: "7000000000000000",
		WebID:    "7600000000000000000",
		FP:       "verify_x",
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestMergeQuotaAPIWindowsAndCards(t *testing.T) {
	summary := []byte(`{"code":0,"data":{"member_info":{"hasActiveSubscription":true},"current_subscription":{"status":4,"display":{"product_name":"个人订阅"}},"window_limit_section":{"window_limit_groups":[{"display":{"title":"滚动用量"},"window_limits":[{"start_time":1700000000,"end_time":1700018000,"used_amount":2,"total_amount":10,"unit":"次"},{"start_time":1700000000,"end_time":1700604800,"used_amount":30,"total_amount":100,"unit":"次"}]}]}}}`)
	balance := []byte(`{"code":0,"data":{"available_count":2,"available_count_by_card_key":{"pc_quota_reset_card":1,"quota_reset_card":1},"has_available_card":true}}`)
	sender := &quotaFakeSender{summary: summary, balance: balance}
	local := pluginapi.QuotaFetchResponse{}
	out := mergeQuotaAPI(pluginapi.QuotaFetchRequest{StorageJSON: testCredentialJSON(t)}, sender, local)
	if out.Subscription == nil || out.Subscription.Plan != "个人订阅" {
		t.Fatalf("unexpected subscription: %+v", out.Subscription)
	}
	var sawFiveHour, sawSevenDay, sawResetCard bool
	for _, group := range out.Groups {
		for _, bucket := range group.Buckets {
			switch bucket.Window {
			case windowFiveHour:
				sawFiveHour = true
				if bucket.RemainingFraction < 0.79 || bucket.RemainingFraction > 0.81 {
					t.Fatalf("unexpected 5h fraction: %v", bucket.RemainingFraction)
				}
				if bucket.ResetTime == "" {
					t.Fatal("5h bucket should carry reset time")
				}
			case windowSevenDay:
				sawSevenDay = true
				if bucket.RemainingFraction < 0.69 || bucket.RemainingFraction > 0.71 {
					t.Fatalf("unexpected 7d fraction: %v", bucket.RemainingFraction)
				}
			case "reset_card":
				sawResetCard = true
				if bucket.RemainingFraction != 1 {
					t.Fatalf("reset card should be available: %v", bucket.RemainingFraction)
				}
			}
		}
	}
	if !sawFiveHour || !sawSevenDay || !sawResetCard {
		t.Fatalf("missing buckets: 5h=%v 7d=%v card=%v %+v", sawFiveHour, sawSevenDay, sawResetCard, out.Groups)
	}
	var cardMetric bool
	for _, metric := range out.Summary {
		if metric.Key == "reset_cards" && metric.Value == 2 {
			cardMetric = true
		}
	}
	if !cardMetric {
		t.Fatalf("missing reset card metric: %+v", out.Summary)
	}
}

func TestQuotaResetUsesAvailableCard(t *testing.T) {
	balance := []byte(`{"code":0,"data":{"available_count":1,"available_count_by_card_key":{"pc_quota_reset_card":1,"quota_reset_card":0},"has_available_card":true}}`)
	sender := &quotaFakeSender{balance: balance, used: []byte(`{"code":0,"msg":"ok"}`)}
	out, err := quotaReset(pluginapi.QuotaResetRequest{StorageJSON: testCredentialJSON(t)}, sender)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Success {
		t.Fatalf("reset not reported as success: %+v", out)
	}
	var body map[string]any
	if json.Unmarshal(sender.lastUse, &body) != nil || body["card_key"] != pcResetCardKey || body["use_request_id"] == "" {
		t.Fatalf("unexpected use payload: %s", sender.lastUse)
	}
}

func TestQuotaResetWithoutCard(t *testing.T) {
	balance := []byte(`{"code":0,"data":{"available_count":0,"available_count_by_card_key":{},"has_available_card":false}}`)
	sender := &quotaFakeSender{balance: balance}
	if _, err := quotaReset(pluginapi.QuotaResetRequest{StorageJSON: testCredentialJSON(t)}, sender); err == nil {
		t.Fatal("expected reset without card to fail")
	}
}
