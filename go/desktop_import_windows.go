//go:build windows

package main

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"database/sql"

	_ "modernc.org/sqlite"
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

var (
	crypt32DLL             = syscall.NewLazyDLL("crypt32.dll")
	procCryptUnprotectData = crypt32DLL.NewProc("CryptUnprotectData")
)

func unprotectDPAPI(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty DPAPI payload")
	}
	var in, out dataBlob
	in.cbData = uint32(len(data))
	in.pbData = &data[0]
	ok, _, callErr := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&out)),
	)
	if ok == 0 {
		return nil, fmt.Errorf("CryptUnprotectData failed: %v", callErr)
	}
	defer syscall.LocalFree(syscall.Handle(unsafe.Pointer(out.pbData)))
	return unsafe.Slice(out.pbData, out.cbData), nil
}

func desktopUserDataDir() (string, error) {
	if custom := os.Getenv("DOUBAO_DESKTOP_USER_DATA"); custom != "" {
		return custom, nil
	}
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return "", errors.New("LOCALAPPDATA is unavailable")
	}
	return filepath.Join(local, "Doubao", "User Data"), nil
}

func readDesktopCookies() (map[string]string, error) {
	userData, err := desktopUserDataDir()
	if err != nil {
		return nil, err
	}
	stateRaw, err := os.ReadFile(filepath.Join(userData, "Local State"))
	if err != nil {
		return nil, errors.New("Doubao desktop client is not installed or has never run")
	}
	var state struct {
		OSCrypt struct {
			EncryptedKey string `json:"encrypted_key"`
		} `json:"os_crypt"`
	}
	if json.Unmarshal(stateRaw, &state) != nil || state.OSCrypt.EncryptedKey == "" {
		return nil, errors.New("Doubao desktop key is unavailable")
	}
	wrapped, err := base64.StdEncoding.DecodeString(state.OSCrypt.EncryptedKey)
	if err != nil || len(wrapped) < 5 || string(wrapped[:5]) != "DPAPI" {
		return nil, errors.New("Doubao desktop key is not DPAPI-wrapped")
	}
	key, err := unprotectDPAPI(wrapped[5:])
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	cookiesPath := filepath.Join(userData, "Default", "Network", "Cookies")
	source, err := os.ReadFile(cookiesPath)
	if err != nil {
		return nil, errors.New("Doubao desktop cookie store is unavailable")
	}
	temp, err := os.CreateTemp("", "doubao-cookies-*.db")
	if err != nil {
		return nil, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err = temp.Write(source); err != nil {
		temp.Close()
		return nil, err
	}
	if err = temp.Close(); err != nil {
		return nil, err
	}

	db, err := openSQLiteReadOnly(tempPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query("select name, encrypted_value from cookies where host_key like '%doubao%' or host_key like '%bytedance%'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cookies := map[string]string{}
	for rows.Next() {
		var name string
		var encrypted []byte
		if rows.Scan(&name, &encrypted) != nil {
			continue
		}
		if len(encrypted) < 3 || string(encrypted[:3]) != "v10" {
			continue
		}
		payload := encrypted[3:]
		if len(payload) < gcm.NonceSize()+gcm.Overhead() {
			continue
		}
		plain, openErr := gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], nil)
		if openErr != nil {
			continue
		}
		if value := asciiTail(plain); value != "" {
			cookies[name] = value
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if cookies["sessionid"] == "" {
		return nil, errors.New("Doubao desktop client is not signed in")
	}
	return cookies, nil
}

// asciiTail drops the short binary prefix the desktop client prepends to each
// decrypted cookie value.
func asciiTail(value []byte) string {
	for start := 0; start < len(value); start++ {
		ok := true
		for _, b := range value[start:] {
			if b < 32 || b > 126 {
				ok = false
				break
			}
		}
		if ok {
			return string(value[start:])
		}
	}
	return ""
}

func openSQLiteReadOnly(path string) (*sql.DB, error) {
	return sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro&immutable=1")
}
