package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
)

const (
	credentialProvider = "doubao"
	entryID            = "doubao"
	defaultBotID       = "7234781073513644036"
)

type credential struct {
	Type     string            `json:"type"`
	Email    string            `json:"email"`
	Cookies  map[string]string `json:"cookies"`
	DeviceID string            `json:"device_id"`
	WebID    string            `json:"web_id"`
	FP       string            `json:"fp"`
	UID      string            `json:"uid,omitempty"`
	LoginAt  int64             `json:"login_at"`
}

func (c credential) cookieHeader() string {
	if len(c.Cookies) == 0 {
		return ""
	}
	names := make([]string, 0, len(c.Cookies))
	for name := range c.Cookies {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		if value := strings.TrimSpace(c.Cookies[name]); value != "" {
			parts = append(parts, name+"="+value)
		}
	}
	return strings.Join(parts, "; ")
}

func readCredential(b []byte) (credential, error) {
	var c credential
	if json.Unmarshal(b, &c) != nil || c.Type != credentialProvider {
		return c, errors.New("invalid doubao credential")
	}
	if strings.TrimSpace(c.Cookies["sessionid"]) == "" {
		return c, errors.New("invalid doubao credential: sessionid missing")
	}
	if c.DeviceID == "" || c.WebID == "" || c.FP == "" {
		return c, errors.New("invalid doubao credential: device identity missing")
	}
	return c, nil
}

func newDeviceIdentity() (deviceID, webID, fp string) {
	chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	part := func(n int) string {
		b := make([]byte, n)
		for i := range b {
			v, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
			if err != nil {
				b[i] = chars[0]
				continue
			}
			b[i] = chars[v.Int64()]
		}
		return string(b)
	}
	fp = fmt.Sprintf("verify_%s_%s_%s_%s_%s", part(8), part(4), part(4), part(4), part(12))
	deviceID = randomDigits(16, '7')
	webID = randomDigits(19, '7')
	if len(deviceID) == 0 || len(webID) == 0 {
		deviceID, webID = "7000000000000000", "7600000000000000000"
	}
	return deviceID, webID, fp
}

func randomDigits(n int, first byte) string {
	out := make([]byte, 0, n)
	for i := 0; i < n; i++ {
		v, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			continue
		}
		d := byte('0' + v.Int64())
		if i == 0 && first != 0 {
			d = byte(first)
		}
		out = append(out, d)
	}
	return string(out)
}
