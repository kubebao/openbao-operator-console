package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

func TestOpenSessionAcceptsOnlyLocalPrincipalsAndRotatesTheSession(t *testing.T) {
	resolve := func(_ context.Context, principal config.Principal) (config.User, bool) {
		return config.User{Subject: principal.Subject}, principal.Issuer == config.LocalIssuer
	}
	a := NewLocal(resolve)
	if a.SingleSignOn() {
		t.Fatal("a console without a provider reported single sign-on")
	}
	request := httptest.NewRequest(http.MethodPost, "https://console.example/api/v1/sign-in/local", nil)
	if err := a.OpenSession(httptest.NewRecorder(), request, config.Principal{Issuer: "https://idp.example", Subject: "x"}); err == nil {
		t.Fatal("a federated principal bypassed the provider login")
	}
	first := httptest.NewRecorder()
	if err := a.OpenSession(first, request, config.Principal{Issuer: config.LocalIssuer, Subject: "account"}); err != nil {
		t.Fatal(err)
	}
	old := first.Result().Cookies()[0]
	again := httptest.NewRequest(http.MethodPost, "https://console.example/api/v1/sign-in/local", nil)
	again.AddCookie(old)
	second := httptest.NewRecorder()
	if err := a.OpenSession(second, again, config.Principal{Issuer: config.LocalIssuer, Subject: "account"}); err != nil {
		t.Fatal(err)
	}
	if _, active := a.logins.lookup(old.Value); active {
		t.Fatal("signing in again kept the previous session")
	}
	login := httptest.NewRecorder()
	a.Login(login, httptest.NewRequest(http.MethodGet, "https://console.example/auth/login", nil))
	if login.Header().Get("Location") != "/" {
		t.Fatal("a local-only console sent sign-in to setup")
	}
	if err := NewDemo(
		config.User{},
	).OpenSession(httptest.NewRecorder(), request, config.Principal{Issuer: config.LocalIssuer}); err == nil {
		t.Fatal("demo mode opened a session")
	}
}
