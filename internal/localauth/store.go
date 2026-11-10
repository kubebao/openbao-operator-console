package localauth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

var (
	// ErrNotFound reports that an account has no stored credential.
	ErrNotFound = errors.New("local credential not found")
	// ErrConflict reports a credential revision that changed concurrently.
	ErrConflict = errors.New("local credential revision changed")
)

// credentialPurpose binds each ciphertext to its account and revision, so a record
// cannot be moved to another account or replayed as a later revision.
const credentialPurpose = "console-local-credential-v1:"

// Credential is the secret part of a local account. The account itself, with its
// username and access, lives in the console configuration. A credential is either
// active, with a password and TOTP secret, or pending, with only an enrollment link.
type Credential struct {
	PasswordHash string `json:"passwordHash,omitempty"`
	TOTPSecret   []byte `json:"totpSecret,omitempty"`
	// TOTPLastStep is the time step of the last accepted code. Codes from that step
	// or earlier are refused.
	TOTPLastStep int64 `json:"totpLastStep"`
	// RecoveryCodes holds SHA-256 digests of the unused recovery codes.
	RecoveryCodes []string `json:"recoveryCodes"`
	// Enrollment is the one-time link of a pending credential. A pending credential
	// cannot sign in.
	Enrollment *PendingEnrollment `json:"enrollment,omitempty"`
}

// PendingEnrollment is a one-time enrollment link that an administrator issued.
type PendingEnrollment struct {
	// TokenDigest is the SHA-256 digest of the link secret.
	TokenDigest string    `json:"tokenDigest"`
	ExpiresAt   time.Time `json:"expiresAt"`
	// TOTPSecret is the authenticator secret offered after the link was opened. It
	// stays the same until enrollment completes, so reloading the page keeps it.
	TOTPSecret []byte `json:"totpSecret,omitempty"`
}

// Pending reports whether the credential waits for enrollment.
func (c Credential) Pending() bool { return c.Enrollment != nil }

func (c Credential) validate() error {
	if c.Enrollment != nil {
		digest, err := hex.DecodeString(c.Enrollment.TokenDigest)
		if err != nil || len(digest) != 32 || c.Enrollment.ExpiresAt.IsZero() || c.PasswordHash != "" ||
			c.TOTPSecret != nil || len(c.RecoveryCodes) != 0 ||
			(c.Enrollment.TOTPSecret != nil && len(c.Enrollment.TOTPSecret) != totpSecretBytes) {
			return errors.New("invalid pending local credential")
		}
		return nil
	}
	if _, _, _, err := decodeHash(c.PasswordHash); err != nil {
		return err
	}
	if len(c.TOTPSecret) != totpSecretBytes || c.TOTPLastStep < 0 || len(c.RecoveryCodes) > RecoveryCodeCount {
		return errors.New("invalid local credential")
	}
	for _, digest := range c.RecoveryCodes {
		if decoded, err := hex.DecodeString(digest); err != nil || len(decoded) != 32 {
			return errors.New("invalid recovery code digest")
		}
	}
	return nil
}

// Record is one encrypted credential row. Revision starts at 1.
type Record struct {
	AccountID string
	Revision  int64
	Payload   []byte
}

// Repository persists encrypted local credentials with revision compare-and-swap.
type Repository interface {
	GetLocalCredential(ctx context.Context, accountID string) (Record, error)
	// CreateLocalCredential stores revision 1 and returns ErrConflict when it exists.
	CreateLocalCredential(ctx context.Context, record Record) error
	// UpdateLocalCredential stores record when the stored revision is record.Revision-1.
	UpdateLocalCredential(ctx context.Context, record Record) error
	// DeleteLocalCredentials removes credentials of accounts that are not in keep.
	DeleteLocalCredentials(ctx context.Context, keep []string) error
}

// Stored is a decrypted credential with its revision.
type Stored struct {
	Revision   int64
	Credential Credential
}

// Store encrypts local credentials with the configuration key.
type Store struct {
	repository Repository
	aead       cipher.AEAD
}

func NewStore(repository Repository, key []byte) (*Store, error) {
	if len(key) != 32 {
		return nil, errors.New("configuration key must contain 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Store{repository: repository, aead: aead}, nil
}

func associated(accountID string, revision int64) []byte {
	return []byte(credentialPurpose + strconv.FormatInt(revision, 10) + ":" + accountID)
}

func (s *Store) seal(accountID string, revision int64, credential Credential) (Record, error) {
	if err := credential.validate(); err != nil {
		return Record{}, err
	}
	plaintext, err := json.Marshal(credential)
	if err != nil {
		return Record{}, err
	}
	defer clear(plaintext)
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return Record{}, err
	}
	payload := s.aead.Seal(nonce, nonce, plaintext, associated(accountID, revision))
	return Record{AccountID: accountID, Revision: revision, Payload: payload}, nil
}

// Load decrypts and validates the credential of accountID.
func (s *Store) Load(ctx context.Context, accountID string) (Stored, error) {
	record, err := s.repository.GetLocalCredential(ctx, accountID)
	if err != nil {
		return Stored{}, err
	}
	size := s.aead.NonceSize()
	if record.AccountID != accountID || len(record.Payload) < size {
		return Stored{}, errors.New("invalid local credential record")
	}
	plaintext, err := s.aead.Open(nil, record.Payload[:size], record.Payload[size:], associated(accountID, record.Revision))
	if err != nil {
		return Stored{}, errors.New("local credential cannot be decrypted with the supplied key")
	}
	defer clear(plaintext)
	var credential Credential
	if err := json.Unmarshal(plaintext, &credential); err != nil {
		return Stored{}, errors.New("invalid local credential record")
	}
	if err := credential.validate(); err != nil {
		return Stored{}, err
	}
	return Stored{Revision: record.Revision, Credential: credential}, nil
}

// Create stores the first credential of a new account.
func (s *Store) Create(ctx context.Context, accountID string, credential Credential) error {
	record, err := s.seal(accountID, 1, credential)
	if err != nil {
		return err
	}
	return s.repository.CreateLocalCredential(ctx, record)
}

// Update replaces the credential stored at revision.
func (s *Store) Update(ctx context.Context, accountID string, revision int64, credential Credential) error {
	record, err := s.seal(accountID, revision+1, credential)
	if err != nil {
		return err
	}
	return s.repository.UpdateLocalCredential(ctx, record)
}

// Prune removes credentials of accounts that no longer exist.
func (s *Store) Prune(ctx context.Context, keep []string) error {
	return s.repository.DeleteLocalCredentials(ctx, keep)
}

// Replace stores credential for accountID whether or not one exists, and returns
// ErrConflict when another writer changed it first.
func (s *Store) Replace(ctx context.Context, accountID string, credential Credential) error {
	current, err := s.Load(ctx, accountID)
	switch {
	case errors.Is(err, ErrNotFound):
		return s.Create(ctx, accountID, credential)
	case err != nil:
		return err
	}
	return s.Update(ctx, accountID, current.Revision, credential)
}
