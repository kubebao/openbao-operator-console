package settings

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"strconv"
)

// Associated data binds each ciphertext to its purpose, record, and revision, so a payload
// cannot be substituted for another record or replayed as a later revision.
const (
	configurationPurpose = "console-configuration-v2:"
	credentialPurpose    = "console-reader-credential-v2:"
)

func configurationData(revision int64) string {
	return configurationPurpose + strconv.FormatInt(revision, 10)
}

// credentialData places the revision before the connection ID. The revision contains no
// colon, so every connection ID and revision pair has distinct associated data.
func credentialData(connectionID string, revision int64) string {
	return credentialPurpose + strconv.FormatInt(revision, 10) + ":" + connectionID
}

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
