package main

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const (
	windowFiveHour = "5h"
	windowSevenDay = "7d"
)

type usageEvent struct {
	At        time.Time
	Fail      bool
	Think     bool
	RequestID string
}

type usageStore struct {
	mu     sync.Mutex
	events map[string][]usageEvent
}

var usageEvents = &usageStore{events: make(map[string][]usageEvent)}

func (s *usageStore) key(authID string) string {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return "default"
	}
	return authID
}

func (s *usageStore) record(authID string, fail bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.key(authID)
	events := append(s.events[key], usageEvent{At: time.Now(), Fail: fail})
	cutoff := time.Now().Add(-8 * 24 * time.Hour)
	kept := make([]usageEvent, 0, len(events))
	for _, event := range events {
		if event.At.After(cutoff) {
			kept = append(kept, event)
		}
	}
	s.events[key] = kept
}

func (s *usageStore) snapshot(authID string) []usageEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.key(authID)
	return append([]usageEvent(nil), s.events[key]...)
}

type windowUsage struct {
	Requests int
	Failures int
	Oldest   time.Time
	Newest   time.Time
}

func windowBounds(events []usageEvent, span time.Duration, now time.Time) windowUsage {
	var out windowUsage
	cutoff := now.Add(-span)
	for _, event := range events {
		if event.At.Before(cutoff) {
			continue
		}
		out.Requests++
		if event.Fail {
			out.Failures++
		}
		if out.Oldest.IsZero() || event.At.Before(out.Oldest) {
			out.Oldest = event.At
		}
		if event.At.After(out.Newest) {
			out.Newest = event.At
		}
	}
	return out
}

// remainingFraction estimates remaining subscription capacity in a rolling
// window. Doubao enforces soft limits, so we translate observed traffic into a
// conservative remaining fraction: five requests per 5h window and thirty per
// 7d window start bending the curve, and hard failures push it toward zero.
func remainingFraction(events []usageEvent, span time.Duration, soft int, now time.Time) float64 {
	usage := windowBounds(events, span, now)
	fraction := 1.0
	if soft > 0 && usage.Requests > 0 {
		fraction = 1.0 - float64(usage.Requests)/float64(soft)
	}
	if usage.Failures > 0 {
		penalty := 0.15 * float64(usage.Failures)
		if penalty > fraction {
			penalty = fraction
		}
		fraction -= penalty
	}
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	return fraction
}

func resetTime(events []usageEvent, span time.Duration, now time.Time) string {
	usage := windowBounds(events, span, now)
	if usage.Oldest.IsZero() {
		return ""
	}
	reset := usage.Oldest.Add(span)
	if reset.Before(now) {
		reset = now.Add(span)
	}
	return reset.UTC().Format(time.RFC3339)
}

func authLabel(storage []byte) string {
	var persisted struct {
		Email string `json:"email"`
	}
	if json.Unmarshal(storage, &persisted) == nil && strings.TrimSpace(persisted.Email) != "" {
		return persisted.Email
	}
	return "豆姐账号"
}

func quotaDescription() pluginapi.QuotaDescribeResponse {
	return pluginapi.QuotaDescribeResponse{
		SupportedProviders: []string{credentialProvider},
		DisplayName:        "Doujie",
		SupportsReset:      true,
	}
}

func localQuotaFetch(req pluginapi.QuotaFetchRequest) (pluginapi.QuotaFetchResponse, error) {
	events := usageEvents.snapshot(req.AuthID)
	now := time.Now()
	fiveHour := windowBounds(events, 5*time.Hour, now)
	sevenDay := windowBounds(events, 7*24*time.Hour, now)
	label := authLabel(req.StorageJSON)
	summary := []pluginapi.QuotaMetric{
		{Key: "requests_5h", Label: "5 小时窗口请求数", Value: float64(fiveHour.Requests), Unit: "requests", Format: "number"},
		{Key: "requests_7d", Label: "7 天窗口请求数", Value: float64(sevenDay.Requests), Unit: "requests", Format: "number"},
		{Key: "failures_7d", Label: "7 天窗口限流/失败", Value: float64(sevenDay.Failures), Unit: "events", Format: "number"},
	}
	if fiveHour.Requests == 0 && sevenDay.Requests == 0 {
		summary = append(summary, pluginapi.QuotaMetric{Key: "note", Label: "状态", Value: 0, Unit: "", Format: "number"})
	}
	out := pluginapi.QuotaFetchResponse{
		Subscription: &pluginapi.QuotaSubscription{
			Plan:     "doubao-personal",
			TierName: label,
		},
		Summary: summary,
		Groups: []pluginapi.QuotaGroup{{
			DisplayName: "豆姐订阅额度（本地估算）",
			Buckets: []pluginapi.QuotaBucket{
				{
					Window:            windowFiveHour,
					RemainingFraction: remainingFraction(events, 5*time.Hour, 5, now),
					ResetTime:         resetTime(events, 5*time.Hour, now),
					Description:       describeBucket(fiveHour, "5 小时滚动窗口（本地估算，重置为窗口起点 + 5h）"),
				},
				{
					Window:            windowSevenDay,
					RemainingFraction: remainingFraction(events, 7*24*time.Hour, 30, now),
					ResetTime:         resetTime(events, 7*24*time.Hour, now),
					Description:       describeBucket(sevenDay, "7 天滚动窗口（本地估算，重置为窗口起点 + 7d）"),
				},
			},
		}},
	}
	return out, nil
}

func describeBucket(usage windowUsage, prefix string) string {
	if usage.Requests == 0 {
		return prefix + "，窗口内暂无请求"
	}
	if usage.Failures > 0 {
		return prefix + "，" + itoa(usage.Requests) + " 次请求 / " + itoa(usage.Failures) + " 次限流或失败"
	}
	return prefix + "，" + itoa(usage.Requests) + " 次请求"
}

func itoa(value int) string {
	digits := ""
	if value == 0 {
		return "0"
	}
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}
