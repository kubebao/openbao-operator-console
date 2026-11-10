package localauth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestEnrollmentLinkIsSingleUseAndRevokesTheOldCredential(t *testing.T) {
	s := newSignIn(t)
	ctx := context.Background()
	link, err := s.authenticator.IssueLink(ctx, "account-alice")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link.Token, "account-alice.") || !link.ExpiresAt.Equal(s.now.Add(EnrollmentLifetime)) {
		t.Fatalf("unexpected link %+v", link)
	}
	// The reset revokes the previous password and authenticator at once.
	s.now = s.now.Add(totpPeriod * time.Second)
	if _, err := s.attempt("maple river lantern 42", totpCode(s.secret, totpStep(s.now))); !errors.Is(err, ErrRejected) {
		t.Fatal("the previous credential still signed in after a reset")
	}
	if _, err := s.attempt("maple river lantern 42", s.codes[1]); !errors.Is(err, ErrRejected) {
		t.Fatal("a previous recovery code still signed in after a reset")
	}
	opened, err := s.authenticator.OpenLink(ctx, link.Token)
	if err != nil || opened.AccountID != "account-alice" || len(opened.TOTPSecret) != totpSecretBytes {
		t.Fatalf("open link: %+v, %v", opened, err)
	}
	again, err := s.authenticator.OpenLink(ctx, link.Token)
	if err != nil || string(again.TOTPSecret) != string(opened.TOTPSecret) {
		t.Fatal("reopening the link changed the authenticator secret")
	}
	if _, _, err := s.authenticator.CompleteLink(
		ctx,
		link.Token,
		"alice",
		"short",
		totpCode(opened.TOTPSecret, totpStep(s.now)),
	); err == nil {
		t.Fatal("a weak password completed enrollment")
	}
	accountID, codes, err := s.authenticator.CompleteLink(
		ctx, link.Token, "alice", "a brand new passphrase here", totpCode(opened.TOTPSecret, totpStep(s.now)),
	)
	if err != nil || accountID != "account-alice" || len(codes) != RecoveryCodeCount {
		t.Fatalf("complete link: %v", err)
	}
	if _, err := s.authenticator.OpenLink(ctx, link.Token); !errors.Is(err, ErrInvalidLink) {
		t.Fatal("a used link opened again")
	}
	s.now = s.now.Add(totpPeriod * time.Second)
	result, err := s.attempt("a brand new passphrase here", totpCode(opened.TOTPSecret, totpStep(s.now)))
	if err != nil || result.RecoveryCodesLeft != RecoveryCodeCount {
		t.Fatalf("sign-in after enrollment: %v", err)
	}
}

func TestEnrollmentLinksRejectForgedReplacedAndExpiredTokens(t *testing.T) {
	s := newSignIn(t)
	ctx := context.Background()
	first, _ := s.authenticator.IssueLink(ctx, "account-alice")
	second, _ := s.authenticator.IssueLink(ctx, "account-alice")
	accountID, secret, _ := strings.Cut(second.Token, ".")
	for name, token := range map[string]string{
		"replaced":      first.Token,
		"forged secret": accountID + "." + strings.Repeat("A", 43),
		"other account": "account-bob." + secret,
		"no separator":  secret,
		"short secret":  accountID + ".abc",
	} {
		if _, err := s.authenticator.OpenLink(ctx, token); !errors.Is(err, ErrInvalidLink) {
			t.Fatalf("%s link: %v", name, err)
		}
	}
	s.now = s.now.Add(EnrollmentLifetime)
	if _, err := s.authenticator.OpenLink(ctx, second.Token); !errors.Is(err, ErrInvalidLink) {
		t.Fatal("an expired link opened")
	}
}

func TestSelfServiceChangesKeepTheAuthenticator(t *testing.T) {
	s := newSignIn(t)
	ctx := context.Background()
	if err := s.authenticator.ChangePassword(ctx, "account-alice", "alice", "alice-password-1"); err == nil {
		t.Fatal("a contextual password was accepted")
	}
	if err := s.authenticator.ChangePassword(ctx, "account-alice", "alice", "copper kettle morning tide"); err != nil {
		t.Fatal(err)
	}
	codes, err := s.authenticator.NewRecoveryCodes(ctx, "account-alice")
	if err != nil || len(codes) != RecoveryCodeCount {
		t.Fatal("recovery codes were not replaced")
	}
	if _, err := s.attempt("copper kettle morning tide", s.codes[0]); !errors.Is(err, ErrRejected) {
		t.Fatal("an old recovery code still worked")
	}
	s.now = s.now.Add(totpPeriod * time.Second)
	if _, err := s.attempt("copper kettle morning tide", totpCode(s.secret, totpStep(s.now))); err != nil {
		t.Fatalf("sign-in with the new password: %v", err)
	}
	if _, err := s.attempt("copper kettle morning tide", codes[0]); err != nil {
		t.Fatalf("sign-in with a new recovery code: %v", err)
	}
}
