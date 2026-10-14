package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
)

// loginData binds sealed login cookies to their purpose.
const loginData = "console-pending-login-v1"

// maxLoginCookie bounds the sealed cookie that the callback accepts.
const maxLoginCookie = 2048

// pendingLogin is a started authorization code flow. It travels in a sealed, short-lived
// cookie instead of server memory, so unauthenticated login requests cannot exhaust a
// server-side table. The state also appears in the authorization request and must match.
type pendingLogin struct {
	State      string `json:"s"`
	Verifier   string `json:"v"`
	Nonce      string `json:"n"`
	ReturnTo   string `json:"r"`
	Expires    int64  `json:"e"`
	Generation int64  `json:"g"`
	// Trial names a server-side candidate provider trial; empty for an ordinary login.
	Trial string `json:"t,omitempty"`
}

// sealer encrypts pending logins with AES-256-GCM under a random per-process key. A
// restart invalidates logins that are in progress, and nothing else.
type sealer struct{ aead cipher.AEAD }

func newSealer() *sealer {
	key := make([]byte, 32)
	// crypto/rand.Read does not return an error; it ends the process if the system
	// random source fails.
	_, _ = rand.Read(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		panic("pending login cipher: " + err.Error())
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic("pending login cipher: " + err.Error())
	}
	return &sealer{aead: aead}
}

func (s *sealer) seal(login pendingLogin) (string, error) {
	plaintext, err := json.Marshal(login)
	if err != nil {
		return "", err
	}
	defer clear(plaintext)
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := s.aead.Seal(nonce, nonce, plaintext, []byte(loginData))
	value := base64.RawURLEncoding.EncodeToString(sealed)
	if len(value) > maxLoginCookie {
		return "", errors.New("pending login exceeds the cookie limit")
	}
	return value, nil
}

// open authenticates and decodes a sealed login. It rejects tampered, foreign, and
// oversized values.
func (s *sealer) open(value string) (pendingLogin, bool) {
	if len(value) > maxLoginCookie {
		return pendingLogin{}, false
	}
	sealed, err := base64.RawURLEncoding.DecodeString(value)
	size := s.aead.NonceSize()
	if err != nil || len(sealed) < size {
		return pendingLogin{}, false
	}
	plaintext, err := s.aead.Open(nil, sealed[:size], sealed[size:], []byte(loginData))
	if err != nil {
		return pendingLogin{}, false
	}
	defer clear(plaintext)
	var login pendingLogin
	if json.Unmarshal(plaintext, &login) != nil || login.State == "" || login.Verifier == "" || login.Nonce == "" {
		return pendingLogin{}, false
	}
	return login, true
}
