package settings

import (
	"crypto/rand"
	"encoding/json"
	"errors"
)

// Associated data binds each ciphertext to its purpose and record, so a payload cannot be
// substituted for another.
const (
	configurationPurpose = "console-configuration-v1"
	credentialPurpose    = "console-reader-credential-v1:"
)

// seal encrypts a JSON value with AES-GCM under a random nonce.
func (s *Service) seal(value any, associated string) ([]byte, error) {
	plaintext, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	defer clear(plaintext)
	nonce := make([]byte, s.cipher.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return s.cipher.Seal(nonce, nonce, plaintext, []byte(associated)), nil
}

func (s *Service) unseal(payload []byte, associated string, value any) error {
	size := s.cipher.NonceSize()
	if len(payload) < size {
		return errors.New("invalid ciphertext")
	}
	plaintext, err := s.cipher.Open(nil, payload[:size], payload[size:], []byte(associated))
	if err != nil {
		return err
	}
	defer clear(plaintext)
	return json.Unmarshal(plaintext, value)
}
