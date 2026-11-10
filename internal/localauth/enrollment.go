package localauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"time"
)

// EnrollmentLifetime is how long an enrollment link stays valid.
const EnrollmentLifetime = 24 * time.Hour

// ErrInvalidLink reports an enrollment link that is unknown, used, replaced, or expired.
var ErrInvalidLink = errors.New("enrollment link is invalid or expired")

// Link is a one-time enrollment link. Token is shown once; only its digest is stored.
type Link struct {
	Token     string
	ExpiresAt time.Time
}

// IssueLink replaces the credential of accountID with a pending one and returns its
// link. The previous password, authenticator, and recovery codes stop working at once.
func (a *Authenticator) IssueLink(ctx context.Context, accountID string) (Link, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return Link{}, err
	}
	secret := base64.RawURLEncoding.EncodeToString(data)
	expires := a.Now().Add(EnrollmentLifetime).UTC().Truncate(time.Second)
	credential := Credential{
		RecoveryCodes: []string{},
		Enrollment:    &PendingEnrollment{TokenDigest: linkDigest(secret), ExpiresAt: expires},
	}
	if err := a.Store.Replace(ctx, accountID, credential); err != nil {
		return Link{}, err
	}
	return Link{Token: accountID + "." + secret, ExpiresAt: expires}, nil
}

func linkDigest(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// LinkAccount returns the account ID that a link token names, without checking it.
func LinkAccount(token string) (string, bool) {
	accountID, _, found := strings.Cut(token, ".")
	return accountID, found && accountID != ""
}

// openLink loads the pending credential that token names and checks the token.
func (a *Authenticator) openLink(ctx context.Context, token string) (string, Stored, error) {
	accountID, secret, found := strings.Cut(token, ".")
	if !found || accountID == "" || len(secret) != 43 {
		return "", Stored{}, ErrInvalidLink
	}
	stored, err := a.Store.Load(ctx, accountID)
	if errors.Is(err, ErrNotFound) {
		return "", Stored{}, ErrInvalidLink
	}
	if err != nil {
		return "", Stored{}, ErrUnavailable
	}
	pending := stored.Credential.Enrollment
	if pending == nil || !a.Now().Before(pending.ExpiresAt) ||
		subtle.ConstantTimeCompare([]byte(linkDigest(secret)), []byte(pending.TokenDigest)) != 1 {
		return "", Stored{}, ErrInvalidLink
	}
	return accountID, stored, nil
}

// OpenedLink is a valid enrollment link with the authenticator secret to show.
type OpenedLink struct {
	AccountID  string
	TOTPSecret []byte
	ExpiresAt  time.Time
}

// OpenLink checks a link and returns the TOTP secret for the account's authenticator.
// The first visit creates and stores the secret; later visits return the same secret.
func (a *Authenticator) OpenLink(ctx context.Context, token string) (OpenedLink, error) {
	accountID, stored, err := a.openLink(ctx, token)
	if err != nil {
		return OpenedLink{}, err
	}
	pending := *stored.Credential.Enrollment
	if pending.TOTPSecret == nil {
		if pending.TOTPSecret, err = NewTOTPSecret(); err != nil {
			return OpenedLink{}, ErrUnavailable
		}
		next := stored.Credential
		next.Enrollment = &pending
		if err := a.Store.Update(ctx, accountID, stored.Revision, next); err != nil {
			// A concurrent visit stored its secret first; use that one.
			return a.reopen(ctx, token, err)
		}
	}
	return OpenedLink{AccountID: accountID, TOTPSecret: slices.Clone(pending.TOTPSecret), ExpiresAt: pending.ExpiresAt}, nil
}

func (a *Authenticator) reopen(ctx context.Context, token string, cause error) (OpenedLink, error) {
	if !errors.Is(cause, ErrConflict) {
		return OpenedLink{}, ErrUnavailable
	}
	accountID, stored, err := a.openLink(ctx, token)
	if err != nil {
		return OpenedLink{}, err
	}
	pending := stored.Credential.Enrollment
	if pending.TOTPSecret == nil {
		return OpenedLink{}, ErrUnavailable
	}
	return OpenedLink{AccountID: accountID, TOTPSecret: slices.Clone(pending.TOTPSecret), ExpiresAt: pending.ExpiresAt}, nil
}

// CompleteLink sets the password and authenticator of the account that token names and
// returns the recovery codes to show once. The link cannot be used again.
func (a *Authenticator) CompleteLink(ctx context.Context, token, username, password, code string) (string, []string, error) {
	accountID, stored, err := a.openLink(ctx, token)
	if err != nil {
		return "", nil, err
	}
	secret := stored.Credential.Enrollment.TOTPSecret
	if secret == nil {
		return "", nil, ErrInvalidLink
	}
	enrollment, err := a.NewEnrollment(ctx, username, password, secret, code)
	if err != nil {
		return "", nil, err
	}
	if err := a.Store.Update(ctx, accountID, stored.Revision, enrollment.Credential); err != nil {
		if errors.Is(err, ErrConflict) {
			return "", nil, ErrInvalidLink
		}
		return "", nil, ErrUnavailable
	}
	return accountID, enrollment.RecoveryCodes, nil
}

// ChangePassword replaces the password of an active credential. The caller verified
// the current password and a second factor with Authenticate.
func (a *Authenticator) ChangePassword(ctx context.Context, accountID, username, password string) error {
	if err := CheckPassword(password, username); err != nil {
		return err
	}
	hash, err := a.Hasher.Hash(ctx, password)
	if err != nil {
		return ErrUnavailable
	}
	return a.updateActive(ctx, accountID, func(credential *Credential) { credential.PasswordHash = hash })
}

// NewRecoveryCodes replaces the recovery codes of an active credential and returns
// the new codes to show once. The caller verified the owner with Authenticate.
func (a *Authenticator) NewRecoveryCodes(ctx context.Context, accountID string) ([]string, error) {
	codes, digests, err := NewRecoveryCodes()
	if err != nil {
		return nil, ErrUnavailable
	}
	if err := a.updateActive(ctx, accountID, func(credential *Credential) { credential.RecoveryCodes = digests }); err != nil {
		return nil, err
	}
	return codes, nil
}

func (a *Authenticator) updateActive(ctx context.Context, accountID string, change func(*Credential)) error {
	for range 3 {
		stored, err := a.Store.Load(ctx, accountID)
		if err != nil || stored.Credential.Pending() {
			return ErrUnavailable
		}
		next := stored.Credential
		next.RecoveryCodes = slices.Clone(next.RecoveryCodes)
		change(&next)
		err = a.Store.Update(ctx, accountID, stored.Revision, next)
		if !errors.Is(err, ErrConflict) {
			if err != nil {
				return ErrUnavailable
			}
			return nil
		}
	}
	return ErrUnavailable
}
