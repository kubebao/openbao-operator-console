package localauth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"strings"
)

const (
	// RecoveryCodeCount is the number of single-use codes issued at enrollment.
	RecoveryCodeCount = 10
	// recoveryCodeBytes gives each code 80 random bits. That makes an unsalted SHA-256
	// digest infeasible to reverse, so Argon2 is not needed for recovery codes.
	recoveryCodeBytes = 10
)

var recoveryEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewRecoveryCodes returns codes to show once and the digests to store.
func NewRecoveryCodes() (codes, digests []string, err error) {
	for range RecoveryCodeCount {
		data := make([]byte, recoveryCodeBytes)
		if _, err := rand.Read(data); err != nil {
			return nil, nil, err
		}
		raw := recoveryEncoding.EncodeToString(data)
		code := raw[0:4] + "-" + raw[4:8] + "-" + raw[8:12] + "-" + raw[12:16]
		codes = append(codes, code)
		digests = append(digests, recoveryDigest(code))
	}
	return codes, digests, nil
}

// recoveryDigest ignores case, spaces, and hyphens, so a code typed from paper matches.
func recoveryDigest(code string) string {
	normalized := strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' {
			return -1
		}
		return r
	}, strings.ToUpper(code))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// matchRecoveryCode returns the index of the stored digest that code matches. It
// compares every digest so that timing does not reveal the position.
func matchRecoveryCode(digests []string, code string) (int, bool) {
	candidate := []byte(recoveryDigest(code))
	matched := -1
	for i, digest := range digests {
		if subtle.ConstantTimeCompare([]byte(digest), candidate) == 1 {
			matched = i
		}
	}
	return matched, matched >= 0
}
