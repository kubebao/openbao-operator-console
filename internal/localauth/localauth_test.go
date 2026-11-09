package localauth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHasherVerifiesNormalizedPasswordsAndFlagsOutdatedParameters(t *testing.T) {
	hasher, err := NewHasher()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// "ﬁ" (U+FB01) normalizes to "fi" under NFKC.
	encoded, err := hasher.Hash(ctx, "a ﬁne long passphrase")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("unexpected encoding %q", encoded)
	}
	if match, rehash, err := hasher.Verify(ctx, encoded, "a fine long passphrase"); err != nil || !match || rehash {
		t.Fatalf("normalized password did not match: %v %v %v", match, rehash, err)
	}
	if match, _, _ := hasher.Verify(ctx, encoded, "a fine long passphrasE"); match {
		t.Fatal("different password matched")
	}
	params, salt, key, err := decodeHash(encoded)
	if err != nil {
		t.Fatal(err)
	}
	params.time = 1
	if _, rehash, _ := hasher.Verify(ctx, encodeHash(params, salt, key), "x"); rehash {
		t.Fatal("rehash was requested for a failed match")
	}
}

func TestDecodeHashRejectsMalformedAndExpensiveParameters(t *testing.T) {
	salt, key := "c2FsdHNhbHRzYWx0c2FsdA", "a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2U"
	for _, encoded := range []string{
		"",
		"$argon2i$v=19$m=19456,t=2,p=1$" + salt + "$" + key,
		"$argon2id$v=16$m=19456,t=2,p=1$" + salt + "$" + key,
		"$argon2id$v=19$m=1048576,t=2,p=1$" + salt + "$" + key,
		"$argon2id$v=19$m=19456,t=100,p=1$" + salt + "$" + key,
		"$argon2id$v=19$m=19456,t=2,p=16$" + salt + "$" + key,
		"$argon2id$v=19$m=19456,t=2,p=1,x=1$" + salt + "$" + key,
		"$argon2id$v=19$m=19456,t=2,p=1$!!$" + key,
		"$argon2id$v=19$m=19456,t=2,p=1$" + salt + "$a2V5",
	} {
		if _, _, _, err := decodeHash(encoded); err == nil {
			t.Fatalf("hash %q was accepted", encoded)
		}
	}
}

func TestCheckPassword(t *testing.T) {
	accepted := []string{
		"maple river lantern 42",
		"Tw0 owls argued about tea",
		"ein langer satz über berge",
		"alice keeps seven blue kites", // contains the username but stays distinctive
	}
	for _, password := range accepted {
		if err := CheckPassword(password, "alice"); err != nil {
			t.Fatalf("%q was rejected: %v", password, err)
		}
	}
	rejected := map[string]PasswordProblem{
		"short phrase":                 PasswordTooShort,
		strings.Repeat("x", 1025):      PasswordTooLong,
		"Password Password":            PasswordCommon,
		"correct horse battery staple": PasswordCommon,
		"aaaaaaaaaaaaaaaaaaaa":         PasswordRepetitive,
		"abcabcabcabcabcabc":           PasswordRepetitive,
		"abcdefghijklmnopq":            PasswordRepetitive,
		"123456789012345":              PasswordRepetitive,
		"qwertyuiopasdfgh":             PasswordRepetitive,
		"alice-openbao-2026!":          PasswordContextual,
		"OpenBao Console Admin 1":      PasswordContextual,
		"aliceconsolealice":            PasswordContextual,
	}
	for password, problem := range rejected {
		var failure PasswordError
		if err := CheckPassword(password, "alice"); !errors.As(err, &failure) || failure.Problem != problem {
			t.Fatalf("%q: got %v, want %s", password, err, problem)
		}
	}
}

func TestTOTPMatchesRFC6238Vectors(t *testing.T) {
	// RFC 6238 appendix B, SHA-1, truncated to six digits.
	secret := []byte("12345678901234567890")
	vectors := map[int64]string{59: "287082", 1111111109: "081804", 1234567890: "005924", 2000000000: "279037"}
	for unix, code := range vectors {
		if got := totpCode(secret, totpStep(time.Unix(unix, 0))); got != code {
			t.Fatalf("time %d: got %s, want %s", unix, got, code)
		}
	}
}

func TestVerifyTOTPAcceptsSkewAndRefusesReplay(t *testing.T) {
	secret := []byte("12345678901234567890")
	now := time.Unix(1234567890, 0)
	step := totpStep(now)
	previous := totpCode(secret, step-1)
	if got, ok := VerifyTOTP(secret, previous, now, 0); !ok || got != step-1 {
		t.Fatal("code from the previous step was refused")
	}
	if _, ok := VerifyTOTP(secret, previous, now, step-1); ok {
		t.Fatal("a code was accepted twice")
	}
	if _, ok := VerifyTOTP(secret, totpCode(secret, step-2), now, 0); ok {
		t.Fatal("code outside the skew window was accepted")
	}
	current := totpCode(secret, step)
	if _, ok := VerifyTOTP(secret, current[:3]+" "+current[3:], now, 0); !ok {
		t.Fatal("code with a space was refused")
	}
	for _, code := range []string{"", "12345", "1234567", "12a456"} {
		if _, ok := VerifyTOTP(secret, code, now, 0); ok {
			t.Fatalf("malformed code %q was accepted", code)
		}
	}
	uri := TOTPURI(secret, "alice")
	if !strings.HasPrefix(uri, "otpauth://totp/OpenBao%20Operator%20Console:alice?") ||
		!strings.Contains(uri, "secret="+TOTPKey(secret)) {
		t.Fatalf("unexpected URI %q", uri)
	}
}

func TestRecoveryCodesAreSingleUseDigests(t *testing.T) {
	codes, digests, err := NewRecoveryCodes()
	if err != nil || len(codes) != RecoveryCodeCount || len(digests) != RecoveryCodeCount {
		t.Fatal("recovery codes were not generated")
	}
	if len(codes[0]) != 19 || strings.Count(codes[0], "-") != 3 {
		t.Fatalf("unexpected code format %q", codes[0])
	}
	if index, ok := matchRecoveryCode(digests, strings.ToLower(strings.ReplaceAll(codes[3], "-", " "))); !ok || index != 3 {
		t.Fatal("recovery code typed with spaces and lowercase was refused")
	}
	if _, ok := matchRecoveryCode(digests, "AAAA-AAAA-AAAA-AAAA"); ok {
		t.Fatal("unknown recovery code matched")
	}
}

func TestThrottleDelaysWithoutLockingOut(t *testing.T) {
	now := time.Unix(0, 0)
	throttle := NewThrottle()
	throttle.now = func() time.Time { return now }
	for range freeFailures - 1 {
		throttle.Failure("alice")
	}
	if _, ok := throttle.Allow("alice"); !ok {
		t.Fatal("attempt within the free failures was delayed")
	}
	throttle.Failure("alice")
	if wait, ok := throttle.Allow("alice"); ok || wait != firstDelay {
		t.Fatalf("got wait %s, allowed %v", wait, ok)
	}
	now = now.Add(firstDelay)
	if _, ok := throttle.Allow("alice"); !ok {
		t.Fatal("attempt after the delay was refused")
	}
	for range 20 {
		throttle.Failure("alice")
	}
	if wait, _ := throttle.Allow("alice"); wait != maxDelay {
		t.Fatalf("delay %s exceeded the cap", wait)
	}
	throttle.Success("alice")
	if _, ok := throttle.Allow("alice"); !ok {
		t.Fatal("success did not clear the delay")
	}
	throttle.Failure("bob")
	now = now.Add(throttleReset)
	if _, ok := throttle.Allow("bob"); !ok {
		t.Fatal("old failures were not forgotten")
	}
}

func TestThrottleEvictionIsBounded(t *testing.T) {
	throttle := NewThrottle()
	base := time.Unix(0, 0)
	for i := range maxThrottled + 10 {
		throttle.now = func() time.Time { return base.Add(time.Duration(i) * time.Millisecond) }
		throttle.Failure(string(rune('a'+i%26)) + strings.Repeat("x", i/26))
	}
	if len(throttle.entries) > maxThrottled {
		t.Fatalf("throttle holds %d entries", len(throttle.entries))
	}
}

type memoryRepository struct {
	mu      sync.Mutex
	records map[string]Record
}

func (r *memoryRepository) GetLocalCredential(_ context.Context, id string) (Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.records[id]
	if !ok {
		return Record{}, ErrNotFound
	}
	return record, nil
}

func (r *memoryRepository) CreateLocalCredential(_ context.Context, record Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.records[record.AccountID]; exists || record.Revision != 1 {
		return ErrConflict
	}
	r.records[record.AccountID] = record
	return nil
}

func (r *memoryRepository) UpdateLocalCredential(_ context.Context, record Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if current, ok := r.records[record.AccountID]; !ok || current.Revision != record.Revision-1 {
		return ErrConflict
	}
	r.records[record.AccountID] = record
	return nil
}

func (r *memoryRepository) DeleteLocalCredentials(_ context.Context, keep []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := map[string]bool{}
	for _, id := range keep {
		kept[id] = true
	}
	for id := range r.records {
		if !kept[id] {
			delete(r.records, id)
		}
	}
	return nil
}

func testKey() []byte { return []byte("0123456789abcdef0123456789abcdef") }

func TestStoreBindsCiphertextToAccountAndRevision(t *testing.T) {
	repository := &memoryRepository{records: map[string]Record{}}
	store, err := NewStore(repository, testKey())
	if err != nil {
		t.Fatal(err)
	}
	hasher, _ := NewHasher()
	hash, _ := hasher.Hash(context.Background(), "maple river lantern 42")
	credential := Credential{PasswordHash: hash, TOTPSecret: make([]byte, totpSecretBytes), RecoveryCodes: []string{}}
	ctx := context.Background()
	if err := store.Create(ctx, "account-a", credential); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, "account-a", credential); !errors.Is(err, ErrConflict) {
		t.Fatal("second create did not conflict")
	}
	stored, err := store.Load(ctx, "account-a")
	if err != nil || stored.Revision != 1 || stored.Credential.PasswordHash != hash {
		t.Fatal("credential did not round-trip")
	}
	// A record copied to another account does not decrypt.
	repository.records["account-b"] = Record{AccountID: "account-b", Revision: 1, Payload: repository.records["account-a"].Payload}
	if _, err := store.Load(ctx, "account-b"); err == nil {
		t.Fatal("credential moved to another account was accepted")
	}
	// A record replayed under a later revision does not decrypt.
	replayed := repository.records["account-a"]
	replayed.Revision = 2
	repository.records["account-a"] = replayed
	if _, err := store.Load(ctx, "account-a"); err == nil {
		t.Fatal("credential replayed under another revision was accepted")
	}
	if err := store.Prune(ctx, []string{"account-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(ctx, "account-b"); !errors.Is(err, ErrNotFound) {
		t.Fatal("orphaned credential was not pruned")
	}
	if err := store.Create(ctx, "account-c", Credential{PasswordHash: "invalid"}); err == nil {
		t.Fatal("invalid credential was stored")
	}
}

type signIn struct {
	authenticator *Authenticator
	secret        []byte
	codes         []string
	now           time.Time
}

func newSignIn(t *testing.T) *signIn {
	t.Helper()
	store, err := NewStore(&memoryRepository{records: map[string]Record{}}, testKey())
	if err != nil {
		t.Fatal(err)
	}
	hasher, err := NewHasher()
	if err != nil {
		t.Fatal(err)
	}
	s := &signIn{now: time.Unix(1_800_000_000, 0)}
	s.authenticator = NewAuthenticator(store, hasher, NewThrottle())
	s.authenticator.now = func() time.Time { return s.now }
	s.authenticator.Throttle.now = func() time.Time { return s.now }
	s.secret, _ = NewTOTPSecret()
	enrollment, err := s.authenticator.NewEnrollment(
		context.Background(), "alice", "maple river lantern 42", s.secret, totpCode(s.secret, totpStep(s.now)),
	)
	if err != nil {
		t.Fatal(err)
	}
	s.codes = enrollment.RecoveryCodes
	if err := store.Create(context.Background(), "account-alice", enrollment.Credential); err != nil {
		t.Fatal(err)
	}
	return s
}

func (s *signIn) attempt(password, code string) (Result, error) {
	return s.authenticator.Authenticate(context.Background(), Attempt{
		Username: "Alice", AccountID: "account-alice", Password: password, Code: code,
	})
}

func TestAuthenticateRequiresPasswordAndAFreshCode(t *testing.T) {
	s := newSignIn(t)
	// The enrollment code's step is used; the next step's code signs in.
	s.now = s.now.Add(totpPeriod * time.Second)
	code := totpCode(s.secret, totpStep(s.now))
	if _, err := s.attempt("maple river lantern 41", code); !errors.Is(err, ErrRejected) {
		t.Fatal("wrong password was accepted")
	}
	if _, err := s.attempt("maple river lantern 42", "000000"); !errors.Is(err, ErrRejected) {
		t.Fatal("wrong code was accepted")
	}
	result, err := s.attempt("maple river lantern 42", code)
	if err != nil || result.RecoveryCodeUsed || result.RecoveryCodesLeft != RecoveryCodeCount {
		t.Fatalf("valid sign-in failed: %v", err)
	}
	if _, err := s.attempt("maple river lantern 42", code); !errors.Is(err, ErrRejected) {
		t.Fatal("a TOTP code completed two sign-ins")
	}
}

func TestAuthenticateConsumesRecoveryCodes(t *testing.T) {
	s := newSignIn(t)
	result, err := s.attempt("maple river lantern 42", s.codes[0])
	if err != nil || !result.RecoveryCodeUsed || result.RecoveryCodesLeft != RecoveryCodeCount-1 {
		t.Fatalf("recovery code sign-in failed: %v", err)
	}
	if _, err := s.attempt("maple river lantern 42", s.codes[0]); !errors.Is(err, ErrRejected) {
		t.Fatal("a recovery code was used twice")
	}
}

func TestAuthenticateThrottlesUnknownAndKnownAccountsAlike(t *testing.T) {
	s := newSignIn(t)
	for range freeFailures {
		_, _ = s.attempt("wrong password value", "000000")
		_, _ = s.authenticator.Authenticate(context.Background(), Attempt{Username: "mallory", Password: "x", Code: "000000"})
	}
	var throttled ThrottledError
	if _, err := s.attempt("maple river lantern 42", totpCode(s.secret, totpStep(s.now)+1)); !errors.As(err, &throttled) {
		t.Fatal("known account was not throttled")
	}
	if _, err := s.authenticator.Authenticate(context.Background(), Attempt{Username: "MALLORY"}); !errors.As(err, &throttled) {
		t.Fatal("unknown account was not throttled")
	}
}

func TestNewEnrollmentRequiresAPolicyPasswordAndAWorkingAuthenticator(t *testing.T) {
	s := newSignIn(t)
	secret, _ := NewTOTPSecret()
	if _, err := s.authenticator.NewEnrollment(
		context.Background(),
		"bob",
		"short",
		secret,
		totpCode(secret, totpStep(s.now)),
	); err == nil {
		t.Fatal("short password was enrolled")
	}
	if _, err := s.authenticator.NewEnrollment(context.Background(), "bob", "maple river lantern 42", secret, "000000"); err == nil {
		t.Fatal("enrollment without a valid code succeeded")
	}
}
