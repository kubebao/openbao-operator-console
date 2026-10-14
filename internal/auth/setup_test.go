package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

func TestPendingLoginAndExpiredSetupCallbackReturnToSetup(t *testing.T) {
	a := NewPending(func(context.Context, config.Principal) (config.User, bool) { return config.User{}, false })
	login := httptest.NewRecorder()
	a.Login(login, httptest.NewRequest(http.MethodGet, "https://console.example/auth/login", nil))
	if login.Code != http.StatusSeeOther || login.Header().Get("Location") != "/setup" {
		t.Fatal("pending console attempted normal authentication")
	}
	activated := false
	trialID, err := a.logins.beginTrial(trial{
		provider: &Provider{},
		setup:    true,
		activate: func(context.Context, config.Principal) error {
			activated = true
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := a.sealer.seal(pendingLogin{
		State:    "expired-state",
		Verifier: "verifier",
		Nonce:    "nonce",
		Trial:    trialID,
		Expires:  time.Now().Add(-time.Minute).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "https://console.example/auth/callback?state=expired-state", nil)
	request.AddCookie(&http.Cookie{Name: "__Host-console-state", Value: sealed})
	for range 2 {
		callback := httptest.NewRecorder()
		a.Callback(callback, request)
		if callback.Code != http.StatusSeeOther || callback.Header().Get("Location") != "/setup?result=expired" || activated {
			t.Fatal("expired or replayed pending callback escaped setup or activated a claim")
		}
	}
	invalid := httptest.NewRecorder()
	a.Callback(invalid, httptest.NewRequest(http.MethodGet, "https://console.example/auth/callback?state=wrong", nil))
	if invalid.Code != http.StatusSeeOther || invalid.Header().Get("Location") != "/setup?result=failed" {
		t.Fatal("pending callback failure exposed provider details or left setup")
	}
}
