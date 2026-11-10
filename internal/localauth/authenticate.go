package localauth

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
)

var (
	// ErrRejected reports a failed sign-in without saying which input was wrong.
	ErrRejected = errors.New("username, password, or code is incorrect")
	// ErrUnavailable reports that sign-in could not be checked, such as a database error.
	ErrUnavailable = errors.New("local sign-in is unavailable")
)

// ThrottledError reports that an account must wait before the next attempt.
type ThrottledError struct{ RetryAfter time.Duration }

func (e ThrottledError) Error() string { return "too many failed sign-ins; wait before trying again" }

// Authenticator verifies a username, password, and second factor together.
type Authenticator struct {
	Store    *Store
	Hasher   *Hasher
	Throttle *Throttle
	// Now returns the time for TOTP checks. Tests replace it.
	Now func() time.Time
}

func NewAuthenticator(store *Store, hasher *Hasher, throttle *Throttle) *Authenticator {
	return &Authenticator{Store: store, Hasher: hasher, Throttle: throttle, Now: time.Now}
}

// Attempt is one sign-in. AccountID is empty when the username is not enrolled.
type Attempt struct {
	Username  string
	AccountID string
	Password  string
	// Code is a six-digit TOTP code or a recovery code.
	Code string
}

// Result describes a successful sign-in.
type Result struct {
	// RecoveryCodeUsed reports that a recovery code replaced the TOTP code.
	RecoveryCodeUsed bool
	// RecoveryCodesLeft is the number of unused recovery codes.
	RecoveryCodesLeft int
}

// ThrottleKey identifies an account for failure delays. It folds case, so variants of
// one username share their delay.
func ThrottleKey(username string) string { return strings.ToLower(strings.TrimSpace(username)) }

// Authenticate checks an attempt. Unknown accounts, wrong passwords, and wrong codes
// all return ErrRejected after the same hashing work. A successful sign-in stores the
// accepted TOTP step or removes the used recovery code with compare-and-swap, so the same
// code cannot complete two sign-ins.
func (a *Authenticator) Authenticate(ctx context.Context, attempt Attempt) (Result, error) {
	key := ThrottleKey(attempt.Username)
	if wait, allowed := a.Throttle.Allow(key); !allowed {
		return Result{}, ThrottledError{RetryAfter: wait}
	}
	if len(attempt.Password) > MaxPasswordBytes || len(attempt.Code) > 64 {
		a.Throttle.Failure(key)
		return Result{}, ErrRejected
	}
	if attempt.AccountID == "" {
		a.Hasher.VerifyAbsent(ctx, attempt.Password)
		a.Throttle.Failure(key)
		return Result{}, ErrRejected
	}
	stored, err := a.Store.Load(ctx, attempt.AccountID)
	if errors.Is(err, ErrNotFound) || err == nil && stored.Credential.Pending() {
		// An account without an active credential has not completed enrollment.
		a.Hasher.VerifyAbsent(ctx, attempt.Password)
		a.Throttle.Failure(key)
		return Result{}, ErrRejected
	}
	if err != nil {
		return Result{}, ErrUnavailable
	}
	match, rehash, err := a.Hasher.Verify(ctx, stored.Credential.PasswordHash, attempt.Password)
	if err != nil {
		return Result{}, ErrUnavailable
	}
	next, result, second := secondFactor(stored.Credential, attempt.Code, a.Now())
	if !match || !second {
		a.Throttle.Failure(key)
		return Result{}, ErrRejected
	}
	if rehash {
		if next.PasswordHash, err = a.Hasher.Hash(ctx, attempt.Password); err != nil {
			return Result{}, ErrUnavailable
		}
	}
	if err := a.Store.Update(ctx, attempt.AccountID, stored.Revision, next); err != nil {
		if errors.Is(err, ErrConflict) {
			// A concurrent sign-in used the credential first. Treat this one as a
			// replay without counting it against the account.
			return Result{}, ErrRejected
		}
		return Result{}, ErrUnavailable
	}
	a.Throttle.Success(key)
	return result, nil
}

// secondFactor checks a TOTP code or a recovery code and returns the credential with
// that code consumed.
func secondFactor(credential Credential, code string, now time.Time) (Credential, Result, bool) {
	next := credential
	next.RecoveryCodes = slices.Clone(credential.RecoveryCodes)
	if _, isTOTP := totpFormat(code); isTOTP {
		step, ok := VerifyTOTP(credential.TOTPSecret, code, now, credential.TOTPLastStep)
		next.TOTPLastStep = step
		return next, Result{RecoveryCodesLeft: len(next.RecoveryCodes)}, ok
	}
	index, ok := matchRecoveryCode(credential.RecoveryCodes, code)
	if !ok {
		return credential, Result{}, false
	}
	next.RecoveryCodes = slices.Delete(next.RecoveryCodes, index, index+1)
	return next, Result{RecoveryCodeUsed: true, RecoveryCodesLeft: len(next.RecoveryCodes)}, true
}

// Enrollment is a new credential and the values shown once to its owner.
type Enrollment struct {
	Credential    Credential
	RecoveryCodes []string
}

// NewEnrollment checks password for username and creates a credential with the given
// TOTP secret. The secret is generated earlier, so the owner can add it to an
// authenticator app and prove it with a code before the credential is stored.
func (a *Authenticator) NewEnrollment(ctx context.Context, username, password string, secret []byte, code string) (Enrollment, error) {
	if err := CheckPassword(password, username); err != nil {
		return Enrollment{}, err
	}
	step, ok := VerifyTOTP(secret, code, a.Now(), -1)
	if !ok {
		return Enrollment{}, ErrRejected
	}
	hash, err := a.Hasher.Hash(ctx, password)
	if err != nil {
		return Enrollment{}, ErrUnavailable
	}
	codes, digests, err := NewRecoveryCodes()
	if err != nil {
		return Enrollment{}, ErrUnavailable
	}
	return Enrollment{
		Credential:    Credential{PasswordHash: hash, TOTPSecret: slices.Clone(secret), TOTPLastStep: step, RecoveryCodes: digests},
		RecoveryCodes: codes,
	}, nil
}
