package main

import "strings"

const (
	siteCN     = "cn"
	siteGlobal = "global"
)

type siteProfile struct {
	ID           string
	DisplayName  string
	Root         string
	PassportAID  string
	ChatAID      string
	DefaultBotID string
	WebVersion   string
	Language     string
	Region       string
	SysRegion    string
}

var siteProfiles = map[string]siteProfile{
	siteCN: {
		ID:           siteCN,
		DisplayName:  "豆姐",
		Root:         "https://www.doubao.com",
		PassportAID:  "497858",
		ChatAID:      "582478",
		DefaultBotID: defaultBotID,
		WebVersion:   "3.45.1",
		Language:     "zh",
		Region:       "CN",
		SysRegion:    "CN",
	},
	siteGlobal: {
		ID:           siteGlobal,
		DisplayName:  "豆姐国际版（Dola）",
		Root:         "https://www.dola.com",
		PassportAID:  "495671",
		ChatAID:      "495671",
		DefaultBotID: "7339470689562525703",
		WebVersion:   "3.44.6",
		Language:     "en",
		Region:       "",
		SysRegion:    "",
	},
}

func normalizeSite(site string) string {
	if strings.EqualFold(strings.TrimSpace(site), siteGlobal) {
		return siteGlobal
	}
	return siteCN
}

func profileForSite(site string) siteProfile {
	return siteProfiles[normalizeSite(site)]
}

func (c credential) site() string {
	return normalizeSite(c.Site)
}

func (c credential) profile() siteProfile {
	return profileForSite(c.site())
}

func (c credential) botID() string {
	cfg := getConfig()
	if c.site() == siteGlobal && cfg.BotID == defaultBotID {
		return c.profile().DefaultBotID
	}
	return cfg.BotID
}
