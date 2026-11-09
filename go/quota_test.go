package main

import (
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

func TestUsageWindowBounds(t *testing.T) {
	now := time.Now()
	events := []usageEvent{
		{At: now.Add(-6 * 24 * time.Hour)},
		{At: now.Add(-2 * time.Hour)},
		{At: now.Add(-1 * time.Hour), Fail: true},
	}
	fiveHour := windowBounds(events, 5*time.Hour, now)
	if fiveHour.Requests != 2 || fiveHour.Failures != 1 {
		t.Fatalf("5h window: %+v", fiveHour)
	}
	sevenDay := windowBounds(events, 7*24*time.Hour, now)
	if sevenDay.Requests != 3 || sevenDay.Failures != 1 {
		t.Fatalf("7d window: %+v", sevenDay)
	}
}

func TestRemainingFractionAndReset(t *testing.T) {
	now := time.Now()
	events := make([]usageEvent, 0, 6)
	for i := 0; i < 3; i++ {
		events = append(events, usageEvent{At: now.Add(-time.Duration(i+1) * 20 * time.Minute)})
	}
	fraction := remainingFraction(events, 5*time.Hour, 5, now)
	if fraction <= 0 || fraction >= 1 {
		t.Fatalf("fraction out of range: %v", fraction)
	}
	reset := resetTime(events, 5*time.Hour, now)
	if reset == "" {
		t.Fatal("reset time should be derived from the oldest event")
	}
	parsed, err := time.Parse(time.RFC3339, reset)
	if err != nil {
		t.Fatalf("reset time not RFC3339: %v", err)
	}
	if !parsed.After(now) {
		t.Fatalf("reset time should be in the future: %v", parsed)
	}
}

func TestQuotaFetchShape(t *testing.T) {
	usageEvents.record("auth-a", false)
	usageEvents.record("auth-a", true)
	resp, err := quotaFetch(pluginapi.QuotaFetchRequest{AuthID: "auth-a", StorageJSON: []byte(`{"email":"tester"}`)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Subscription == nil || resp.Subscription.Plan != "doubao-personal" {
		t.Fatalf("unexpected subscription: %+v", resp.Subscription)
	}
	if len(resp.Groups) != 1 || len(resp.Groups[0].Buckets) != 2 {
		t.Fatalf("unexpected groups: %+v", resp.Groups)
	}
	if resp.Groups[0].Buckets[0].Window != windowFiveHour || resp.Groups[0].Buckets[1].Window != windowSevenDay {
		t.Fatalf("unexpected windows: %+v", resp.Groups[0].Buckets)
	}
	if resp.Groups[0].Buckets[1].RemainingFraction >= 1 {
		t.Fatalf("failure should reduce remaining fraction: %+v", resp.Groups[0].Buckets[1])
	}
	if len(resp.Summary) < 3 {
		t.Fatalf("expected summary metrics: %+v", resp.Summary)
	}
}

func TestQuotaDescription(t *testing.T) {
	desc := quotaDescription()
	if desc.DisplayName != "Doujie" || !desc.SupportsReset {
		t.Fatalf("unexpected description: %+v", desc)
	}
	if len(desc.SupportedProviders) != 1 || desc.SupportedProviders[0] != credentialProvider {
		t.Fatalf("unexpected providers: %+v", desc.SupportedProviders)
	}
}
