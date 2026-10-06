package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPendingLoginAndExpiredSetupCallbackReturnToSetup(t *testing.T) {
	a := NewPending()
	login := httptest.NewRecorder()
	a.Login(login, httptest.NewRequest("GET", "https://console.example/auth/login", nil))
	if login.Code != http.StatusSeeOther || login.Header().Get("Location") != "/setup" {
		t.Fatal("pending console attempted normal authentication")
	}
	activated := false
	a.transactions["expired-state"] = transaction{setup: true, expires: time.Now().Add(-time.Minute), activate: func(context.Context, string, []string) error {
		activated = true
		return nil
	}}
	request := httptest.NewRequest("GET", "https://console.example/auth/callback?state=expired-state", nil)
	request.AddCookie(&http.Cookie{Name: "__Host-console-state", Value: "expired-state"})
	for range 2 {
		callback := httptest.NewRecorder()
		a.Callback(callback, request)
		if callback.Code != http.StatusSeeOther || callback.Header().Get("Location") != "/setup?result=expired" || activated {
			t.Fatal("expired or replayed pending callback escaped setup or activated a claim")
		}
	}
	invalid := httptest.NewRecorder()
	a.Callback(invalid, httptest.NewRequest("GET", "https://console.example/auth/callback?state=wrong", nil))
	if invalid.Code != http.StatusSeeOther || invalid.Header().Get("Location") != "/setup?result=failed" {
		t.Fatal("pending callback failure exposed provider details or left setup")
	}
}
