package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const (
	catalogTTL        = 10 * time.Minute
	catalogFailureTTL = 45 * time.Second
)

type flexString string

func (f *flexString) UnmarshalJSON(raw []byte) error {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		*f = flexString(strings.TrimSpace(text))
		return nil
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		*f = flexString(number.String())
		return nil
	}
	return fmt.Errorf("unsupported string value %s", string(raw))
}

type catalogEffortLevel struct {
	Level int    `json:"level"`
	Name  string `json:"name"`
}

type catalogReasoningConfig struct {
	DefaultLevel int                  `json:"default_level"`
	OptionList   []catalogEffortLevel `json:"option_list"`
}

type catalogSubscribeConfig struct {
	NeedUpgrade bool `json:"need_upgrade"`
}

type catalogModel struct {
	ModelItemKey          flexString              `json:"model_item_key"`
	ItemID                flexString              `json:"item_id"`
	Name                  string                  `json:"name"`
	SubTitleName          string                  `json:"sub_title_name"`
	DefaultMode           flexString              `json:"default_mode"`
	ModelExtraParams      map[string]any          `json:"model_extra_params"`
	ReasoningEffortConfig *catalogReasoningConfig `json:"reasoning_effort_config"`
	ConversationModeList  []string                `json:"conversation_mode_list"`
	SubscribeConfig       *catalogSubscribeConfig `json:"subscribe_config"`
}

type catalogModelList struct {
	ItemList []catalogModel `json:"item_list"`
}

type catalogSupportModel struct {
	ItemID       flexString `json:"item_id"`
	ModelItemKey flexString `json:"model_item_key"`
}

type catalogMode struct {
	ModeID               flexString            `json:"mode_id"`
	Name                 string                `json:"name"`
	AgentMode            int                   `json:"agent_mode"`
	ShowReasoningEffort  *bool                 `json:"show_reasoning_effort"`
	ModelList            catalogModelList      `json:"model_list"`
	SupportModels        []catalogSupportModel `json:"support_models"`
	ConversationModeList []string              `json:"conversation_mode_list"`
}

type catalogMenu struct {
	UseModeModelList bool `json:"use_mode_model_list"`
	ModeList         struct {
		ItemList []catalogMode `json:"item_list"`
	} `json:"mode_list"`
	ModelList catalogModelList `json:"model_list"`
}

type catalogSwitchConfig struct {
	SwitchType    int         `json:"switch_type"`
	UseMenuConfV2 bool        `json:"use_menu_conf_v2"`
	MenuConfV2    catalogMenu `json:"menu_conf_v2"`
	MenuConf      catalogMenu `json:"menu_conf"`
}

type catalogEntry struct {
	ActionType       int                  `json:"action_type"`
	ActionBarKey     string               `json:"action_bar_key"`
	ActiveSwitchConf *catalogSwitchConfig `json:"active_switch_conf"`
}

type catalogBriefResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		EntryList []catalogEntry `json:"entry_list"`
	} `json:"data"`
}

type catalogSpec struct {
	ID            string
	Display       string
	Description   string
	Selection     modelSelection
	Context       int64
	Output        int64
	Efforts       []int
	EffortNames   map[int]string
	DefaultEffort int
	Available     bool
	Version       string
}

type catalogCacheEntry struct {
	specs   []catalogSpec
	expires time.Time
	err     error
}

var (
	catalogCacheMu   sync.Mutex
	catalogCache     = map[string]catalogCacheEntry{}
	catalogFetchLock = map[string]*sync.Mutex{}
)

var catalogDateSuffix = regexp.MustCompile(`-(?:20\d{6}|\d{6})$`)

// Stable public IDs for menu keys that already existed in the plugin. These
// values are API contracts: clients persist them, so do not rename casually.
var catalogIDAliases = map[string]string{
	"9":                       "doubao",
	"5":                       "doubao-pro",
	"3":                       "doubao-seed-1.6",
	"0":                       "doubao-seed-1.6-flash",
	"seed-lite-7b":            "doubao-seed-2.0-lite",
	"deepseek-v4-1-flash":     "deepseek-v4.1-flash",
	"deepseek-v4-pro-ga":      "deepseek-v4-pro",
	"kimi-k3-1-pro":           "kimi-k3.1-pro",
	"kimi-k3-1-pro-highspeed": "kimi-k3.1-pro-high-speed",
	"kimi-k3":                 "kimi-k3",
	"glm-5-3":                 "glm-5.3",
	"glm-5-3-flash":           "glm-5.3-flash",
	"qwen4-max":               "qwen4-max",
	"qwen3-8-max":             "qwen3-8-max",
	"qwen3-8-flash":           "qwen3-8-flash",
}

func catalogCacheKey(c credential) string {
	identity := strings.TrimSpace(c.UID)
	if identity == "" {
		identity = strings.TrimSpace(c.Cookies["sessionid"])
	}
	return strings.TrimSpace(c.DeviceID) + "|" + strings.TrimSpace(c.WebID) + "|" + identity
}

func catalogLockFor(key string) *sync.Mutex {
	catalogCacheMu.Lock()
	defer catalogCacheMu.Unlock()
	lock := catalogFetchLock[key]
	if lock == nil {
		lock = &sync.Mutex{}
		catalogFetchLock[key] = lock
	}
	return lock
}

func storeCatalogCache(key string, specs []catalogSpec, err error, ttl time.Duration) {
	catalogCacheMu.Lock()
	defer catalogCacheMu.Unlock()
	catalogCache[key] = catalogCacheEntry{specs: specs, expires: time.Now().Add(ttl), err: err}
}

func cachedCatalog(c credential) ([]catalogSpec, bool) {
	if c.site() != siteCN {
		return nil, false
	}
	catalogCacheMu.Lock()
	defer catalogCacheMu.Unlock()
	entry, ok := catalogCache[catalogCacheKey(c)]
	if !ok || time.Now().After(entry.expires) || entry.err != nil || len(entry.specs) == 0 {
		return nil, false
	}
	return entry.specs, true
}

// catalogForCredential returns the live menu for a domestic credential. Both
// successful and failed lookups are cached so model-list refreshes cannot
// hammer the upstream endpoint.
func catalogForCredential(c credential, sender httpSender) ([]catalogSpec, error) {
	if c.site() != siteCN {
		return nil, fmt.Errorf("dynamic model catalog is only available for the domestic site")
	}
	key := catalogCacheKey(c)
	now := time.Now()

	catalogCacheMu.Lock()
	cached, ok := catalogCache[key]
	catalogCacheMu.Unlock()
	if ok && now.Before(cached.expires) {
		if cached.err != nil {
			return nil, cached.err
		}
		if len(cached.specs) > 0 {
			return cached.specs, nil
		}
	}

	lock := catalogLockFor(key)
	lock.Lock()
	defer lock.Unlock()

	now = time.Now()
	catalogCacheMu.Lock()
	cached, ok = catalogCache[key]
	catalogCacheMu.Unlock()
	if ok && now.Before(cached.expires) {
		if cached.err != nil {
			return nil, cached.err
		}
		if len(cached.specs) > 0 {
			return cached.specs, nil
		}
	}

	if sender == nil {
		err := fmt.Errorf("model catalog transport is unavailable")
		storeCatalogCache(key, nil, err, catalogFailureTTL)
		return nil, err
	}
	specs, err := fetchCatalog(c, sender)
	if err != nil {
		storeCatalogCache(key, nil, err, catalogFailureTTL)
		return nil, err
	}
	storeCatalogCache(key, specs, nil, catalogTTL)
	return specs, nil
}

func fetchCatalog(c credential, sender httpSender) ([]catalogSpec, error) {
	profile := c.profile()
	body, err := json.Marshal(map[string]string{
		"bot_id":        modelSelectionBotID,
		"language_code": profile.Language,
	})
	if err != nil {
		return nil, err
	}
	params := securityParams(c, "catalog")
	// The v2 menu is gated on a modern web client version. The desktop
	// app_version (2.1.7) returns the legacy menu without model entries.
	params.Set("pc_version", profile.WebVersion)
	params.Set("doubao_pc_version", profile.WebVersion)
	target := profile.Root + "/alice/slot/action_bar_v3/brief_list?" + params.Encode()

	headers := http.Header{
		"Accept":          {"application/json, text/plain, */*"},
		"Content-Type":    {"application/json"},
		"Agw-Js-Conv":     {"str"},
		"User-Agent":      {chromeUA},
		"Referer":         {profile.Root + "/chat/"},
		"Origin":          {profile.Root},
		"Accept-Language": {profile.Language + ",en;q=0.8"},
		"Cookie":          {c.cookieHeader()},
	}
	if csrf := firstNonBlank(c.Cookies["passport_csrf_token"], c.Cookies["passport_csrf_token_default"]); csrf != "" {
		headers.Set("X-Tt-Passport-Csrf-Token", csrf)
	}
	resp, err := sender.Do(pluginapi.HTTPRequest{
		Method:  http.MethodPost,
		URL:     target,
		Headers: headers,
		Body:    body,
	})
	if err != nil {
		return nil, fmt.Errorf("model catalog request failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("model catalog request returned status %d", resp.StatusCode)
	}
	return parseCatalogResponse(resp.Body)
}

func parseCatalogResponse(body []byte) ([]catalogSpec, error) {
	var resp catalogBriefResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode model catalog: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("model catalog returned code %d: %s", resp.Code, resp.Msg)
	}
	var conf *catalogSwitchConfig
	for i := range resp.Data.EntryList {
		entry := &resp.Data.EntryList[i]
		if entry.ActionType != 11 || entry.ActiveSwitchConf == nil {
			continue
		}
		if entry.ActiveSwitchConf.SwitchType == 1 || entry.ActionBarKey == "coco_deep_thinking" {
			conf = entry.ActiveSwitchConf
			break
		}
	}
	if conf == nil {
		return nil, fmt.Errorf("model catalog menu entry was not found")
	}
	if !conf.UseMenuConfV2 {
		return nil, fmt.Errorf("model catalog v2 menu is not enabled for this account")
	}
	menu := conf.MenuConfV2
	if len(menu.ModeList.ItemList) == 0 && len(menu.ModelList.ItemList) == 0 {
		return nil, fmt.Errorf("model catalog menu is empty")
	}

	specs := make([]catalogSpec, 0, 16)
	seen := map[string]int{}
	add := func(model catalogModel, mode *catalogMode) {
		spec, ok := catalogSpecFrom(model, mode)
		if !ok {
			return
		}
		if index, exists := seen[spec.ID]; exists {
			if preferCatalogSpec(spec, specs[index]) {
				specs[index] = spec
			}
			return
		}
		seen[spec.ID] = len(specs)
		specs = append(specs, spec)
	}

	if len(menu.ModeList.ItemList) > 0 {
		for i := range menu.ModeList.ItemList {
			mode := &menu.ModeList.ItemList[i]
			models := mode.ModelList.ItemList
			if len(models) == 0 {
				models = menu.ModelList.ItemList
			}
			for _, model := range models {
				add(model, mode)
			}
		}
	} else {
		for _, model := range menu.ModelList.ItemList {
			add(model, nil)
		}
	}
	if len(specs) == 0 {
		return nil, fmt.Errorf("model catalog has no available models")
	}
	return specs, nil
}

func catalogSpecFrom(model catalogModel, mode *catalogMode) (catalogSpec, bool) {
	key := strings.TrimSpace(string(model.ModelItemKey))
	itemID := strings.TrimSpace(string(model.ItemID))
	if key == "" {
		return catalogSpec{}, false
	}
	if model.SubscribeConfig != nil && model.SubscribeConfig.NeedUpgrade {
		return catalogSpec{}, false
	}
	id := stableCatalogModelID(key, itemID, model.Name)
	if id == "" {
		return catalogSpec{}, false
	}
	modeID := strings.TrimSpace(string(model.DefaultMode))
	agentMode := 1
	conversationMode := ""
	if mode != nil {
		if value := strings.TrimSpace(string(mode.ModeID)); value != "" {
			modeID = value
		}
		if mode.AgentMode != 0 {
			agentMode = mode.AgentMode
		}
		conversationMode = conversationModeValue(modeID, mode.ConversationModeList, model.ConversationModeList)
	} else {
		conversationMode = conversationModeValue(modeID, model.ConversationModeList)
	}
	if modeID == "" {
		modeID = "3"
	}
	if conversationMode == "" {
		conversationMode = "2"
	}

	efforts, names, defaultEffort := catalogEffortConfig(model.ReasoningEffortConfig)
	if defaultEffort == 0 {
		return catalogSpec{}, false
	}
	context := extraInt(model.ModelExtraParams, "total_window_size")
	return catalogSpec{
		ID:          id,
		Display:     catalogDisplayName(id, model.Name),
		Description: strings.TrimSpace(model.SubTitleName),
		Selection: modelSelection{
			ItemKey:          key,
			ItemID:           itemID,
			ModeID:           modeID,
			ConversationMode: conversationMode,
			AgentMode:        agentMode,
			ReasoningEffort:  defaultEffort,
			Context:          context,
			Efforts:          efforts,
			DefaultEffort:    defaultEffort,
			ExtraParams:      normalizedExtraParams(model.ModelExtraParams),
		},
		Context:       context,
		Output:        0,
		Efforts:       efforts,
		EffortNames:   names,
		DefaultEffort: defaultEffort,
		Available:     true,
		Version:       catalogKeyVersion(key),
	}, true
}

func preferCatalogSpec(candidate, current catalogSpec) bool {
	if candidate.Available != current.Available {
		return candidate.Available
	}
	return candidate.Version > current.Version
}

func catalogEffortConfig(config *catalogReasoningConfig) ([]int, map[int]string, int) {
	if config == nil {
		return nil, nil, 0
	}
	levels := make([]int, 0, len(config.OptionList))
	names := map[int]string{}
	for _, option := range config.OptionList {
		if option.Level <= 0 {
			continue
		}
		if _, exists := names[option.Level]; !exists {
			levels = append(levels, option.Level)
		}
		if name := effortLevelName(option.Level); name != "" {
			names[option.Level] = name
		} else if name := strings.TrimSpace(option.Name); name != "" {
			names[option.Level] = name
		}
	}
	sort.Ints(levels)
	defaultLevel := config.DefaultLevel
	if !containsInt(levels, defaultLevel) {
		if len(levels) > 0 {
			defaultLevel = levels[0]
		} else {
			defaultLevel = 0
		}
	}
	return levels, names, defaultLevel
}

func conversationModeValue(modeID string, lists ...[]string) string {
	switch strings.TrimSpace(modeID) {
	case "1":
		return "1"
	case "3":
		return "2"
	}
	for _, list := range lists {
		for _, raw := range list {
			switch strings.ToLower(strings.TrimSpace(raw)) {
			case "chat":
				return "1"
			case "office", "creation", "task":
				return "2"
			}
		}
	}
	return ""
}

func normalizedExtraParams(input map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range input {
		switch typed := value.(type) {
		case string:
			out[key] = typed
		case float64:
			out[key] = strconv.FormatFloat(typed, 'f', -1, 64)
		case json.Number:
			out[key] = typed.String()
		case bool:
			out[key] = strconv.FormatBool(typed)
		case nil:
			continue
		default:
			out[key] = fmt.Sprint(typed)
		}
	}
	return out
}

func extraInt(input map[string]any, key string) int64 {
	value, ok := input[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	case float64:
		return int64(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	default:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(typed)), 10, 64)
		return parsed
	}
}

func stableCatalogModelID(key, itemID, name string) string {
	raw := strings.ToLower(strings.TrimSpace(key))
	if id, ok := catalogIDAliases[raw]; ok {
		return id
	}
	base := normalizeCatalogKey(raw)
	if id, ok := catalogIDAliases[base]; ok {
		return id
	}
	slug := slugASCII(base)
	if id, ok := catalogIDAliases[slug]; ok {
		return id
	}
	if slug == "" || isDigits(slug) {
		slug = slugASCII(name)
	}
	if slug == "" {
		if itemID = strings.TrimSpace(itemID); itemID != "" {
			slug = "doubao-" + itemID
		}
	}
	return slug
}

func normalizeCatalogKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.TrimPrefix(key, "person-")
	return catalogDateSuffix.ReplaceAllString(key, "")
}

func catalogKeyVersion(key string) string {
	match := catalogDateSuffix.FindString(strings.ToLower(strings.TrimSpace(key)))
	return strings.TrimPrefix(match, "-")
}

func slugASCII(value string) string {
	var out strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && out.Len() > 0 {
			out.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(out.String(), "-")
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func catalogDisplayName(id, fallback string) string {
	if id == "doubao" {
		return "豆姐 Auto"
	}
	if fallback = strings.TrimSpace(fallback); fallback != "" {
		return fallback
	}
	return id
}

func catalogModelInfo(spec catalogSpec) pluginapi.ModelInfo {
	output := spec.Output
	input := spec.Context - output
	if input < 1 {
		input = spec.Context
	}
	return pluginapi.ModelInfo{
		ID:                         prefixedModelID(spec.ID),
		Object:                     "model",
		OwnedBy:                    credentialProvider,
		Type:                       credentialProvider,
		Name:                       prefixedModelID(spec.ID),
		DisplayName:                spec.Display,
		ContextLength:              spec.Context,
		InputTokenLimit:            input,
		MaxCompletionTokens:        output,
		OutputTokenLimit:           output,
		Version:                    spec.Version,
		Description:                spec.Description,
		SupportedGenerationMethods: []string{"chat"},
		SupportedInputModalities:   []string{"text"},
		SupportedParameters:        []string{"prompt", "ratio", "seconds", "resolution"},
		Thinking:                   thinkingForEfforts(spec.Efforts, spec.EffortNames),
	}
}

func thinkingForEfforts(levels []int, names map[int]string) *pluginapi.ThinkingSupport {
	if len(levels) == 0 {
		return nil
	}
	out := make([]string, 0, len(levels))
	for _, level := range levels {
		name := strings.TrimSpace(names[level])
		if name == "" {
			name = effortLevelName(level)
		}
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil
	}
	return &pluginapi.ThinkingSupport{Levels: out}
}

func effortLevelName(level int) string {
	switch level {
	case 2:
		return "minimal"
	case 3:
		return "low"
	case 4:
		return "medium"
	case 5:
		return "high"
	case 6:
		return "xhigh"
	case 7:
		return "max"
	default:
		return ""
	}
}

func parseEffortLevel(value string) int {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "minimal":
		return 2
	case "low":
		return 3
	case "medium":
		return 4
	case "high":
		return 5
	case "xhigh", "extra-high", "extra high":
		return 6
	case "max":
		return 7
	}
	if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
		return parsed
	}
	return 0
}

func (s modelSelection) effectiveEffort(requested string) int {
	level := parseEffortLevel(requested)
	if level <= 0 {
		if s.DefaultEffort > 0 {
			return s.DefaultEffort
		}
		return s.ReasoningEffort
	}
	if len(s.Efforts) == 0 {
		return level
	}
	if containsInt(s.Efforts, level) {
		return level
	}
	if s.DefaultEffort > 0 {
		return s.DefaultEffort
	}
	return s.ReasoningEffort
}

func selectionForModelForCredential(c credential, model string) (modelSelection, bool) {
	specs, ok := cachedCatalog(c)
	if !ok {
		return modelSelection{}, false
	}
	return lookupCatalogSelection(specs, model)
}

func selectionForRequest(c credential, model string, sender httpSender) (modelSelection, bool) {
	if selection, ok := selectionForModelForCredential(c, model); ok {
		return selection, true
	}
	if c.site() == siteCN && sender != nil {
		if _, err := catalogForCredential(c, sender); err == nil {
			return selectionForModelForCredential(c, model)
		}
	}
	return modelSelection{}, false
}

func lookupCatalogSelection(specs []catalogSpec, model string) (modelSelection, bool) {
	id := strings.ToLower(cleanModelID(model))
	rawKey := strings.TrimSpace(model)
	normalizedKey := normalizeCatalogKey(model)
	for _, spec := range specs {
		if strings.EqualFold(spec.ID, id) {
			return spec.Selection, true
		}
		if rawKey != "" && strings.EqualFold(spec.Selection.ItemKey, rawKey) {
			return spec.Selection, true
		}
		if normalizedKey != "" && normalizeCatalogKey(spec.Selection.ItemKey) == normalizedKey {
			return spec.Selection, true
		}
	}
	return modelSelection{}, false
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
