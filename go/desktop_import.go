package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
)

const desktopEntryID = "doubao-desktop"

// importDesktopAuth builds a credential from the local Doubao desktop client.
// The platform-specific reader returns the session cookies; this function
// normalizes them into the same auth shape as a QR sign-in.
func importDesktopAuth(sender httpSender, cookies map[string]string) (pluginapi.AuthData, error) {
	deviceID, webID, fp := newDeviceIdentity()
	c := credential{
		Type:     credentialProvider,
		Site:     siteCN,
		Cookies:  cookies,
		DeviceID: deviceID,
		WebID:    webID,
		FP:       fp,
		LoginAt:  time.Now().Unix(),
	}
	label := desktopAccountLabel(sender, c)
	c.Email = label
	raw, err := json.Marshal(c)
	if err != nil {
		return pluginapi.AuthData{}, err
	}
	sum := sha256.Sum256([]byte(cookies["sessionid"]))
	id := "doubao-" + hex.EncodeToString(sum[:12]) + ".json"
	return pluginapi.AuthData{
		Provider:    credentialProvider,
		ID:          id,
		FileName:    id,
		Label:       label,
		StorageJSON: raw,
		Metadata: map[string]any{
			"email":          label,
			"type":           credentialProvider,
			"site":           siteCN,
			"oauth_provider": desktopEntryID,
			"source":         "desktop-import",
		},
	}, nil
}

func desktopAccountLabel(sender httpSender, c credential) string {
	fallback := "豆姐账号（桌面端导入）"
	if sender == nil {
		return fallback
	}
	target := doubaoRoot + "/passport/account/info/v2/?aid=" + passportAID
	headers := browserHeaders(nil)
	headers.Set("Cookie", c.cookieHeader())
	resp, err := sender.Do(pluginapi.HTTPRequest{Method: "GET", URL: target, Headers: headers})
	if err != nil || resp.StatusCode != 200 {
		return fallback
	}
	var envelope struct {
		Data struct {
			Name       string `json:"name"`
			ScreenName string `json:"screen_name"`
		} `json:"data"`
	}
	if json.Unmarshal(resp.Body, &envelope) != nil {
		return fallback
	}
	name := strings.TrimSpace(envelope.Data.ScreenName)
	if name == "" {
		name = strings.TrimSpace(envelope.Data.Name)
	}
	if name == "" {
		return fallback
	}
	return "豆姐账号 " + name
}
