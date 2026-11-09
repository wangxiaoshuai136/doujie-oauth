//go:build windows

package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"database/sql"

	"golang.org/x/sys/windows"
	_ "modernc.org/sqlite"
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

var (
	crypt32DLL             = syscall.NewLazyDLL("crypt32.dll")
	procCryptUnprotectData = crypt32DLL.NewProc("CryptUnprotectData")
	advapi32DLL            = syscall.NewLazyDLL("advapi32.dll")
	procImpersonateUser    = advapi32DLL.NewProc("ImpersonateLoggedOnUser")
	procRevertToSelf       = advapi32DLL.NewProc("RevertToSelf")
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
	plain := append([]byte(nil), unsafe.Slice(out.pbData, out.cbData)...)
	_, _ = syscall.LocalFree(syscall.Handle(unsafe.Pointer(out.pbData)))
	return plain, nil
}

func readDesktopCookies() (map[string]string, error) {
	if custom := strings.TrimSpace(os.Getenv("DOUBAO_DESKTOP_USER_DATA")); custom != "" {
		return readDesktopCookiesFromDir(custom)
	}
	local := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	var currentErr error
	if local != "" {
		cookies, err := readDesktopCookiesFromDir(filepath.Join(local, "Doubao", "User Data"))
		if err == nil {
			return cookies, nil
		}
		currentErr = err
	} else {
		currentErr = errors.New("LOCALAPPDATA is unavailable")
	}
	if !processRunsInSessionZero() {
		return nil, currentErr
	}
	// CPA Core is commonly installed as a LocalSystem service. DPAPI keys are
	// scoped to the interactive user, so impersonate the active desktop session
	// before touching the Doubao profile instead of requiring a QR sign-in.
	cookies, err := readDesktopCookiesAsConsoleUser()
	if err != nil {
		return nil, fmt.Errorf("%v; cannot read the active desktop session: %w", currentErr, err)
	}
	return cookies, nil
}

func readDesktopCookiesFromDir(userData string) (map[string]string, error) {
	stateRaw, err := os.ReadFile(filepath.Join(userData, "Local State"))
	if err != nil {
		return nil, fmt.Errorf("Doubao desktop client is not installed or has never run: %w", err)
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
		return nil, fmt.Errorf("Doubao desktop cookie store is unavailable: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(cookiesPath), "doubao-cookies-*.db")
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
	rows, err := db.Query("select host_key, name, encrypted_value from cookies where host_key like '%doubao%' or host_key like '%bytedance%'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cookies := map[string]string{}
	sessionSeen := false
	for rows.Next() {
		var hostKey string
		var name string
		var encrypted []byte
		if rows.Scan(&hostKey, &name, &encrypted) != nil {
			continue
		}
		if name == "sessionid" {
			sessionSeen = true
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
		if value := cookiePlaintext(hostKey, plain); value != "" {
			cookies[name] = value
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if cookies["sessionid"] == "" {
		if sessionSeen {
			return nil, errors.New("Doubao desktop session cookie could not be decrypted")
		}
		return nil, errors.New("Doubao desktop client is not signed in")
	}
	return cookies, nil
}

func processRunsInSessionZero() bool {
	var sessionID uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &sessionID); err != nil {
		return false
	}
	return sessionID == 0
}

func readDesktopCookiesAsConsoleUser() (map[string]string, error) {
	token, profile, err := activeConsoleUser()
	if err != nil {
		return nil, err
	}
	defer token.Close()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ok, _, callErr := procImpersonateUser.Call(uintptr(token))
	if ok == 0 {
		return nil, fmt.Errorf("ImpersonateLoggedOnUser failed: %v", callErr)
	}
	defer procRevertToSelf.Call()

	userData := filepath.Join(profile, "AppData", "Local", "Doubao", "User Data")
	return readDesktopCookiesFromDir(userData)
}

func activeConsoleUser() (windows.Token, string, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, "", err
	}
	defer windows.CloseHandle(snapshot)

	activeSession := windows.WTSGetActiveConsoleSessionId()
	var fallbackPID uint32
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err = windows.Process32First(snapshot, &entry); err != nil {
		return 0, "", err
	}
	for {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "explorer.exe") {
			var sessionID uint32
			if windows.ProcessIdToSessionId(entry.ProcessID, &sessionID) == nil && sessionID != 0 {
				if activeSession == 0xffffffff || sessionID == activeSession {
					return tokenForProcess(entry.ProcessID)
				}
				if fallbackPID == 0 {
					fallbackPID = entry.ProcessID
				}
			}
		}
		if err = windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				break
			}
			return 0, "", err
		}
	}
	if fallbackPID != 0 {
		return tokenForProcess(fallbackPID)
	}
	return 0, "", errors.New("no signed-in Windows desktop session is available")
}

func tokenForProcess(pid uint32) (windows.Token, string, error) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, "", err
	}
	defer windows.CloseHandle(process)
	var source windows.Token
	if err = windows.OpenProcessToken(process, windows.TOKEN_QUERY|windows.TOKEN_DUPLICATE, &source); err != nil {
		return 0, "", err
	}
	defer source.Close()
	profile, err := windowsUserProfileDir(source)
	if err != nil {
		return 0, "", err
	}
	var token windows.Token
	if err = windows.DuplicateTokenEx(source, windows.TOKEN_QUERY|windows.TOKEN_IMPERSONATE|windows.TOKEN_DUPLICATE, nil, windows.SecurityImpersonation, windows.TokenImpersonation, &token); err != nil {
		return 0, "", err
	}
	return token, profile, nil
}

func windowsUserProfileDir(token windows.Token) (string, error) {
	size := uint32(windows.MAX_PATH)
	buf := make([]uint16, size)
	err := windows.GetUserProfileDirectory(token, &buf[0], &size)
	if err != nil && size > uint32(len(buf)) {
		buf = make([]uint16, size)
		err = windows.GetUserProfileDirectory(token, &buf[0], &size)
	}
	if err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf), nil
}

func cookiePlaintext(hostKey string, value []byte) string {
	prefix := sha256.Sum256([]byte(hostKey))
	if len(value) >= len(prefix) && bytes.Equal(value[:len(prefix)], prefix[:]) {
		value = value[len(prefix):]
	}
	return printableTail(value)
}

// printableTail handles older cookie stores that used a short binary prefix
// instead of Chromium's SHA256(host_key) binding prefix.
func printableTail(value []byte) string {
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
