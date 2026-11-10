package localauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6238 HMAC-SHA1 is what authenticator apps support; HMAC-SHA1 remains secure.
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TOTP follows RFC 6238 with the parameters that authenticator apps support everywhere.
const (
	totpSecretBytes = 20
	totpDigits      = 6
	totpPeriod      = 30
	// totpSkew accepts one step on either side of the current one for clock drift.
	totpSkew = 1
	// TOTPIssuer labels the account in authenticator apps.
	TOTPIssuer = "OpenBao Operator Console"
)

var totpEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret returns a random 160-bit TOTP secret.
func NewTOTPSecret() ([]byte, error) {
	secret := make([]byte, totpSecretBytes)
	_, err := rand.Read(secret)
	return secret, err
}

// TOTPKey returns the secret in the base32 form that authenticator apps accept for
// manual entry.
func TOTPKey(secret []byte) string { return totpEncoding.EncodeToString(secret) }

// TOTPURI returns the otpauth URI that a QR code carries for enrollment.
func TOTPURI(secret []byte, account string) string {
	label := url.PathEscape(TOTPIssuer + ":" + account)
	query := url.Values{
		"secret":    {TOTPKey(secret)},
		"issuer":    {TOTPIssuer},
		"algorithm": {"SHA1"},
		"digits":    {strconv.Itoa(totpDigits)},
		"period":    {strconv.Itoa(totpPeriod)},
	}
	return "otpauth://totp/" + label + "?" + query.Encode()
}

func totpStep(now time.Time) int64 { return now.Unix() / totpPeriod }

func totpCode(secret []byte, step int64) string {
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step)) //nolint:gosec // Steps after 1970 are positive.
	mac := hmac.New(sha1.New, secret)
	mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, value%1_000_000)
}

// TOTPCode returns the code for secret at time at. Tests use it to act as an
// authenticator app.
func TOTPCode(secret []byte, at time.Time) string { return totpCode(secret, totpStep(at)) }

// totpFormat reports whether code has the shape of a TOTP code after removing spaces.
func totpFormat(code string) (string, bool) {
	code = strings.ReplaceAll(code, " ", "")
	if len(code) != totpDigits {
		return "", false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return "", false
		}
	}
	return code, true
}

// VerifyTOTP returns the time step that code matches. A code is accepted once: its
// step must be later than last, the step of the previously accepted code.
func VerifyTOTP(secret []byte, code string, now time.Time, last int64) (int64, bool) {
	code, ok := totpFormat(code)
	if !ok || len(secret) != totpSecretBytes {
		return 0, false
	}
	current := totpStep(now)
	matched := int64(-1)
	for step := current - totpSkew; step <= current+totpSkew; step++ {
		// Compare every candidate so that timing does not reveal which step matched.
		if subtle.ConstantTimeCompare([]byte(totpCode(secret, step)), []byte(code)) == 1 && step > last {
			matched = step
		}
	}
	return matched, matched >= 0
}
