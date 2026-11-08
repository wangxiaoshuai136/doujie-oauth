package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var stateKeyMu sync.Mutex

func loadStateKey(authDir string) ([]byte, error) {
	if authDir == "" {
		return nil, errors.New("CPA auth directory unavailable")
	}
	stateKeyMu.Lock()
	defer stateKeyMu.Unlock()
	if err := os.MkdirAll(authDir, 0700); err != nil {
		return nil, errors.New("cannot create auth directory")
	}
	path := filepath.Join(authDir, ".doubao-oauth-state.key")
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != 32 {
			return nil, errors.New("invalid state encryption key")
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, errors.New("cannot read state encryption key")
	}
	key = make([]byte, 32)
	if _, err = io.ReadFull(rand.Reader, key); err != nil {
		return nil, errors.New("cannot create state encryption key")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		key, err = os.ReadFile(path)
		if err != nil || len(key) != 32 {
			return nil, errors.New("cannot read state encryption key")
		}
		return key, nil
	}
	if err != nil {
		return nil, errors.New("cannot persist state encryption key")
	}
	_, err = f.Write(key)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		_ = os.Remove(path)
		return nil, errors.New("cannot persist state encryption key")
	}
	return key, nil
}

func sealState(key []byte, flowID string, expires time.Time) (string, error) {
	if len(flowID) != 32 || expires.Unix() <= 0 {
		return "", errors.New("unsupported login flow state")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	plain := make([]byte, 9+len(flowID))
	plain[0] = 1
	binary.BigEndian.PutUint64(plain[1:9], uint64(expires.Unix()))
	copy(plain[9:], flowID)
	return base64.RawURLEncoding.EncodeToString(append(nonce, gcm.Seal(nil, nonce, plain, []byte("doubao-login-v1"))...)), nil
}

func openState(key []byte, encoded string) (string, time.Time, error) {
	var zero time.Time
	if len(encoded) > 160 {
		return "", zero, errors.New("invalid login state")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(raw) < 12+9+16 {
		return "", zero, errors.New("invalid login state")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", zero, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", zero, err
	}
	nonce := raw[:gcm.NonceSize()]
	p, err := gcm.Open(nil, nonce, raw[gcm.NonceSize():], []byte("doubao-login-v1"))
	if err != nil || len(p) < 9 || p[0] != 1 {
		return "", zero, errors.New("invalid login state")
	}
	expires := time.Unix(int64(binary.BigEndian.Uint64(p[1:9])), 0)
	flowID := string(p[9:])
	if len(flowID) != 32 {
		return "", zero, errors.New("invalid login state")
	}
	return flowID, expires, nil
}
