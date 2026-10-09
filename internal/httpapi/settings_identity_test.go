package httpapi

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/kubebao/openbao-operator-console/internal/auth"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

func testProviderCA(provider *httptest.Server) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: provider.Certificate().Raw}))
}

func testAuthenticator(t *testing.T, provider *httptest.Server, service *settings.Service) *auth.Authenticator {
	t.Helper()
	a, err := auth.NewOIDC(oidc.ClientContext(context.Background(), provider.Client()), provider.URL, "console", "secret", "https://console.example/auth/callback", nil)
	if err != nil {
		t.Fatal(err)
	}
	a.SetResolver(service.Resolve)
	return a
}

func completeTestLogin(t *testing.T, provider *httptest.Server, a *auth.Authenticator, location string, cookies []*http.Cookie) *http.Cookie {
	t.Helper()
	client := provider.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Get(location)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	callbackURL, err := url.Parse(response.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, callbackURL.String(), nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	callback := httptest.NewRecorder()
	a.Callback(callback, request)
	if callback.Code != 303 {
		t.Fatalf("callback failed: %d %s", callback.Code, callback.Body.String())
	}
	for _, cookie := range callback.Result().Cookies() {
		if cookie.Name == "__Host-console-session" {
			return cookie
		}
	}
	t.Fatal("callback did not issue a session")
	return nil
}
