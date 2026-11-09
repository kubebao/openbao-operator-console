// Package localauth verifies local console accounts: Argon2id passwords, TOTP codes,
// recovery codes, and per-account failure delays. OpenID Connect remains the recommended
// sign-in; local accounts serve the first administrator and small installations.
package localauth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
)

// Argon2id parameters follow the OWASP minimum: 19 MiB, two passes, one lane.
const (
	argonMemory  = 19 * 1024 // KiB
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16
	// maxConcurrentHashes bounds hashing memory to about 76 MiB.
	maxConcurrentHashes = 4
	// Stored hashes with larger parameters are refused, so a tampered record cannot
	// make one verification exhaust memory or time.
	maxArgonMemory  = 256 * 1024
	maxArgonTime    = 10
	maxArgonThreads = 4
)

var errEncodedHash = errors.New("invalid password hash")

// Hasher hashes and verifies passwords with Argon2id. It bounds concurrent hashing.
type Hasher struct {
	slots chan struct{}
	// dummy is verified for unknown accounts, so their response time matches known ones.
	dummy string
}

func NewHasher() (*Hasher, error) {
	h := &Hasher{slots: make(chan struct{}, maxConcurrentHashes)}
	dummy, err := h.Hash(context.Background(), "console-unknown-account-placeholder")
	if err != nil {
		return nil, err
	}
	h.dummy = dummy
	return h, nil
}

func (h *Hasher) acquire(ctx context.Context) error {
	select {
	case h.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *Hasher) release() { <-h.slots }

// Hash returns the PHC-encoded Argon2id hash of the normalized password.
func (h *Hasher) Hash(ctx context.Context, password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	if err := h.acquire(ctx); err != nil {
		return "", err
	}
	defer h.release()
	key := argon2.IDKey([]byte(Normalize(password)), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return encodeHash(argonParams{memory: argonMemory, time: argonTime, threads: argonThreads}, salt, key), nil
}

// Verify reports whether password matches encoded. Rehash reports that the stored hash
// uses outdated parameters and should be replaced after a successful sign-in.
func (h *Hasher) Verify(ctx context.Context, encoded, password string) (match, rehash bool, err error) {
	params, salt, expected, err := decodeHash(encoded)
	if err != nil {
		return false, false, err
	}
	if err := h.acquire(ctx); err != nil {
		return false, false, err
	}
	defer h.release()
	keyLen := uint32(len(expected)) //nolint:gosec // decodeHash bounds the key to 64 bytes.
	key := argon2.IDKey([]byte(Normalize(password)), salt, params.time, params.memory, params.threads, keyLen)
	match = subtle.ConstantTimeCompare(key, expected) == 1
	current := argonParams{memory: argonMemory, time: argonTime, threads: argonThreads}
	return match, match && (params != current || len(salt) != argonSaltLen || len(expected) != argonKeyLen), nil
}

// VerifyAbsent spends the same work as Verify for an unknown account and never matches.
func (h *Hasher) VerifyAbsent(ctx context.Context, password string) {
	_, _, _ = h.Verify(ctx, h.dummy, password)
}

// Normalize applies Unicode NFKC, so equivalent input from different keyboards and
// platforms produces the same password.
func Normalize(password string) string { return norm.NFKC.String(password) }

type argonParams struct {
	memory  uint32
	time    uint32
	threads uint8
}

func encodeHash(params argonParams, salt, key []byte) string {
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		params.memory,
		params.time,
		params.threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)
}

func decodeHash(encoded string) (argonParams, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return argonParams{}, nil, nil, errEncodedHash
	}
	var params argonParams
	var threads uint32
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.memory, &params.time, &threads); err != nil ||
		fmt.Sprintf("m=%d,t=%d,p=%d", params.memory, params.time, threads) != parts[3] {
		return argonParams{}, nil, nil, errEncodedHash
	}
	if params.memory < 8*threads || params.memory > maxArgonMemory || params.time < 1 || params.time > maxArgonTime ||
		threads < 1 || threads > maxArgonThreads {
		return argonParams{}, nil, nil, errEncodedHash
	}
	params.threads = uint8(threads)
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return argonParams{}, nil, nil, errEncodedHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) < 16 || len(key) > 64 {
		return argonParams{}, nil, nil, errEncodedHash
	}
	return params, salt, key, nil
}
