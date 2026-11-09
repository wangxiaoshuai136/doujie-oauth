package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

// Doubao exposes its subscription quota and reset-card balance through the
// commerce endpoints used by the web/desktop account panel. They accept the
// session cookies directly and do not require the chat request signature.
const (
	quotaSummaryPath = "/alice/commerce/sale/subscription/quota/summary/"
	cardBalancePath  = "/alice/commerce/marketing/card/balance/"
	cardUsePath      = "/alice/commerce/marketing/card/use/"
	cardListPath     = "/alice/commerce/marketing/card/list/"
	pcResetCardKey   = "pc_quota_reset_card"
	resetCardKey     = "quota_reset_card"
)

type flexNumber float64

func (f *flexNumber) UnmarshalJSON(raw []byte) error {
	text := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	text = strings.ReplaceAll(text, ",", "")
	text = strings.ReplaceAll(text, "，", "")
	text = strings.TrimSpace(text)
	if text == "" || text == "null" {
		*f = 0
		return nil
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		*f = 0
		return nil
	}
	*f = flexNumber(value)
	return nil
}

type windowLimit struct {
	StartTime        flexNumber `json:"start_time"`
	EndTime          flexNumber `json:"end_time"`
	UsedAmount       flexNumber `json:"used_amount"`
	TotalAmount      flexNumber `json:"total_amount"`
	UsedPercent      flexNumber `json:"used_percent"`
	TotalUsedPercent flexNumber `json:"total_used_percent"`
	Unit             string     `json:"unit"`
}

type windowGroup struct {
	Title   string `json:"title"`
	Display struct {
		Title string `json:"title"`
	} `json:"display"`
	WindowLimits []windowLimit `json:"window_limits"`
}

type windowSection struct {
	EntitlementCount  flexNumber    `json:"entitlement_count"`
	UsageExhausted    bool          `json:"usage_exhausted"`
	WindowLimitGroups []windowGroup `json:"window_limit_groups"`
}

type quotaSummaryResponse struct {
	Code    flexNumber `json:"code"`
	Msg     string     `json:"msg"`
	Message string     `json:"message"`
	Data    struct {
		CurrentSubscription struct {
			SkuKey  string `json:"sku_key"`
			Status  int    `json:"status"`
			Display struct {
				ProductName string `json:"product_name"`
			} `json:"display"`
		} `json:"current_subscription"`
		MemberInfo struct {
			HasActiveSubscription     bool `json:"hasActiveSubscription"`
			HasEnterpriseSubscription bool `json:"hasEnterpriseSubscription"`
		} `json:"member_info"`
		ServiceCurrentTime           flexNumber    `json:"service_current_time"`
		WindowLimitSection           windowSection `json:"window_limit_section"`
		EnterpriseWindowLimitSection windowSection `json:"enterprise_window_limit_section"`
		IncentiveQuotaSection        windowSection `json:"incentive_quota_section"`
	} `json:"data"`
}

type cardBalanceResponse struct {
	Code    flexNumber `json:"code"`
	Msg     string     `json:"msg"`
	Message string     `json:"message"`
	Data    struct {
		AvailableCount          flexNumber            `json:"available_count"`
		AvailableCountByCardKey map[string]flexNumber `json:"available_count_by_card_key"`
		HasAvailableCard        bool                  `json:"has_available_card"`
		HideEntry               bool                  `json:"hide_entry"`
	} `json:"data"`
}

type cardUseResponse struct {
	Code    flexNumber `json:"code"`
	Msg     string     `json:"msg"`
	Message string     `json:"message"`
}

func commerceParams(c credential) strings.Builder {
	profile := c.profile()
	params := make([]string, 0, 24)
	add := func(key, value string) {
		if value != "" {
			params = append(params, key+"="+urlQueryEscape(value))
		}
	}
	add("version_code", "20800")
	add("language", profile.Language)
	add("device_platform", "web")
	add("doubao_device_platform", "web")
	add("aid", profile.PassportAID)
	add("real_aid", profile.PassportAID)
	add("pkg_type", "release_version")
	add("device_id", c.DeviceID)
	add("pc_version", profile.WebVersion)
	add("doubao_pc_version", profile.WebVersion)
	add("web_id", c.WebID)
	add("tea_uuid", c.WebID)
	add("region", profile.Region)
	add("sys_region", profile.SysRegion)
	add("samantha_web", "1")
	add("web_platform", "browser")
	add("use-olympus-account", "1")
	add("web_tab_id", randomUUID())
	if token := strings.TrimSpace(c.Cookies["msToken"]); token != "" {
		add("msToken", token)
	}
	var out strings.Builder
	for index, item := range params {
		if index > 0 {
			out.WriteByte('&')
		}
		out.WriteString(item)
	}
	return out
}

func urlQueryEscape(value string) string {
	return strings.NewReplacer(
		"%", "%25", " ", "%20", "+", "%2B", "&", "%26", "=", "%3D",
		"?", "%3F", "#", "%23", "/", "%2F", ":", "%3A",
	).Replace(value)
}

func quotaAPIRequest(sender httpSender, c credential, path string, body any) ([]byte, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, errors.New("cannot encode quota request")
	}
	params := commerceParams(c)
	target := c.profile().Root + path
	if params.Len() > 0 {
		target += "?" + params.String()
	}
	headers := http.Header{
		"Accept":          {"application/json"},
		"Content-Type":    {"application/json"},
		"User-Agent":      {chromeUA},
		"Referer":         {c.profile().Root + "/chat/"},
		"Origin":          {c.profile().Root},
		"Accept-Language": {c.profile().Language + ",en;q=0.8"},
		"Cookie":          {c.cookieHeader()},
	}
	resp, err := sender.Do(pluginapi.HTTPRequest{Method: http.MethodPost, URL: target, Headers: headers, Body: raw})
	if err != nil {
		return nil, errors.New("quota service unreachable")
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, errors.New("session expired")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("quota service returned HTTP %d", resp.StatusCode)
	}
	if len(resp.Body) > 4<<20 {
		return nil, errors.New("quota response too large")
	}
	return resp.Body, nil
}

func fetchQuotaSummary(sender httpSender, c credential) (quotaSummaryResponse, error) {
	var out quotaSummaryResponse
	body, err := quotaAPIRequest(sender, c, quotaSummaryPath, map[string]any{"product_line": "membership"})
	if err != nil {
		return out, err
	}
	if json.Unmarshal(body, &out) != nil {
		return out, errors.New("invalid quota summary")
	}
	if int(out.Code) != 0 && int(out.Code) != 200 {
		return out, errors.New(firstNonEmpty(out.Msg, out.Message, "quota summary rejected"))
	}
	return out, nil
}

func fetchCardBalance(sender httpSender, c credential) (cardBalanceResponse, error) {
	var out cardBalanceResponse
	body, err := quotaAPIRequest(sender, c, cardBalancePath, map[string]any{})
	if err != nil {
		return out, err
	}
	if json.Unmarshal(body, &out) != nil {
		return out, errors.New("invalid card balance")
	}
	if int(out.Code) != 0 && int(out.Code) != 200 {
		return out, errors.New(firstNonEmpty(out.Msg, out.Message, "card balance rejected"))
	}
	return out, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func toUnixSeconds(value flexNumber) int64 {
	raw := int64(value)
	if raw <= 0 {
		return 0
	}
	if raw > 1_000_000_000_000 {
		return raw / 1000
	}
	return raw
}

func windowLabel(limit windowLimit) string {
	start := toUnixSeconds(limit.StartTime)
	end := toUnixSeconds(limit.EndTime)
	if start > 0 && end > start {
		duration := end - start
		switch {
		case duration >= 4*3600 && duration <= 6*3600:
			return "5h"
		case duration >= 6*24*3600 && duration <= 8*24*3600:
			return "7d"
		}
	}
	return "window"
}

func windowRemaining(limit windowLimit) float64 {
	total := float64(limit.TotalAmount)
	used := float64(limit.UsedAmount)
	fraction := 1.0
	switch {
	case total > 0:
		fraction = 1 - used/total
	case float64(limit.TotalUsedPercent) > 0:
		fraction = 1 - float64(limit.TotalUsedPercent)/100
	case float64(limit.UsedPercent) > 0:
		fraction = 1 - float64(limit.UsedPercent)/100
	}
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	return fraction
}

func windowDescription(limit windowLimit) string {
	total := float64(limit.TotalAmount)
	used := float64(limit.UsedAmount)
	if total > 0 {
		unit := strings.TrimSpace(limit.Unit)
		return fmt.Sprintf("已用 %s / %s%s", formatAmount(used), formatAmount(total), unit)
	}
	if float64(limit.TotalUsedPercent) > 0 {
		return fmt.Sprintf("已用 %.0f%%", float64(limit.TotalUsedPercent))
	}
	if float64(limit.UsedPercent) > 0 {
		return fmt.Sprintf("已用 %.0f%%", float64(limit.UsedPercent))
	}
	return "窗口内暂无用量"
}

func formatAmount(value float64) string {
	if value == float64(int64(value)) {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', 1, 64)
}

func bucketsFromSection(section windowSection) []pluginapi.QuotaBucket {
	var buckets []pluginapi.QuotaBucket
	for _, group := range section.WindowLimitGroups {
		for _, limit := range group.WindowLimits {
			end := toUnixSeconds(limit.EndTime)
			reset := ""
			if end > 0 {
				reset = time.Unix(end, 0).UTC().Format(time.RFC3339)
			}
			description := windowDescription(limit)
			if title := firstNonEmpty(group.Display.Title, group.Title); title != "" {
				description = title + " · " + description
			}
			buckets = append(buckets, pluginapi.QuotaBucket{
				Window:            windowLabel(limit),
				RemainingFraction: windowRemaining(limit),
				ResetTime:         reset,
				Description:       description,
			})
		}
	}
	return buckets
}

func cardMetricValue(balance cardBalanceResponse) int {
	total := 0
	for _, value := range balance.Data.AvailableCountByCardKey {
		total += int(value)
	}
	if total == 0 {
		total = int(balance.Data.AvailableCount)
	}
	return total
}

func mergeQuotaAPI(req pluginapi.QuotaFetchRequest, sender httpSender, local pluginapi.QuotaFetchResponse) pluginapi.QuotaFetchResponse {
	c, err := readCredential(req.StorageJSON)
	if err != nil {
		return local
	}
	summary, summaryErr := fetchQuotaSummary(sender, c)
	balance, balanceErr := fetchCardBalance(sender, c)

	if summaryErr == nil {
		buckets := bucketsFromSection(summary.Data.WindowLimitSection)
		if len(buckets) == 0 {
			buckets = bucketsFromSection(summary.Data.EnterpriseWindowLimitSection)
		}
		if len(buckets) == 0 {
			buckets = bucketsFromSection(summary.Data.IncentiveQuotaSection)
		}
		if len(buckets) > 0 {
			local.Groups = []pluginapi.QuotaGroup{{DisplayName: "豆姐订阅额度", Buckets: buckets}}
			local.Summary = nil
		}
		if summary.Data.MemberInfo.HasActiveSubscription || summary.Data.CurrentSubscription.Status != 0 {
			plan := firstNonEmpty(summary.Data.CurrentSubscription.Display.ProductName, summary.Data.CurrentSubscription.SkuKey)
			if plan == "" {
				plan = "豆姐订阅"
			}
			tier := ""
			if local.Subscription != nil {
				tier = local.Subscription.TierName
			}
			local.Subscription = &pluginapi.QuotaSubscription{Plan: plan, TierName: tier}
		}
	}

	if balanceErr == nil {
		cards := cardMetricValue(balance)
		local.Summary = append(local.Summary, pluginapi.QuotaMetric{
			Key:    "reset_cards",
			Label:  "可用重置卡",
			Value:  float64(cards),
			Unit:   "cards",
			Format: "number",
		})
		fraction := 0.0
		if cards > 0 {
			fraction = 1
		}
		local.Groups = append(local.Groups, pluginapi.QuotaGroup{
			DisplayName: "额度重置卡",
			Buckets: []pluginapi.QuotaBucket{{
				Window:            "reset_card",
				RemainingFraction: fraction,
				Description:       fmt.Sprintf("可用 %d 张（pc %d / 通用 %d）", cards, int(balance.Data.AvailableCountByCardKey[pcResetCardKey]), int(balance.Data.AvailableCountByCardKey[resetCardKey])),
			}},
		})
	}
	return local
}

func quotaFetch(req pluginapi.QuotaFetchRequest, sender httpSender) (pluginapi.QuotaFetchResponse, error) {
	local, err := localQuotaFetch(req)
	if err != nil {
		return pluginapi.QuotaFetchResponse{}, err
	}
	if sender == nil {
		return local, nil
	}
	return mergeQuotaAPI(req, sender, local), nil
}

func quotaReset(req pluginapi.QuotaResetRequest, sender httpSender) (pluginapi.QuotaResetResponse, error) {
	var out pluginapi.QuotaResetResponse
	c, err := readCredential(req.StorageJSON)
	if err != nil {
		return out, errors.New("invalid Doubao credential")
	}
	balance, err := fetchCardBalance(sender, c)
	if err != nil {
		return out, err
	}
	key := ""
	if int(balance.Data.AvailableCountByCardKey[pcResetCardKey]) > 0 {
		key = pcResetCardKey
	} else if int(balance.Data.AvailableCountByCardKey[resetCardKey]) > 0 {
		key = resetCardKey
	} else if int(balance.Data.AvailableCount) > 0 {
		key = resetCardKey
	}
	if key == "" {
		return out, errors.New("当前账号没有可用的重置卡")
	}
	body, err := quotaAPIRequest(sender, c, cardUsePath, map[string]any{
		"card_key":       key,
		"use_request_id": randomUUID(),
		"trigger_scene":  "",
		"card_id":        "",
	})
	if err != nil {
		return out, err
	}
	var used cardUseResponse
	if json.Unmarshal(body, &used) != nil {
		return out, errors.New("invalid reset response")
	}
	if int(used.Code) != 0 && int(used.Code) != 200 {
		return out, errors.New(firstNonEmpty(used.Msg, used.Message, "重置卡使用失败"))
	}
	return pluginapi.QuotaResetResponse{Success: true, Message: "已使用重置卡：" + key}, nil
}
