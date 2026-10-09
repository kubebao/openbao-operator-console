package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

func TestCredentialStatusesAuthorizeAndProjectOnlyLifecycleEvidence(t *testing.T) {
	service := configurationService(t)
	for _, auth := range []viewerAuth{{allowed: false}, {allowed: true, user: config.User{Subject: "reader"}}} {
		handler := (&Server{Repository: &memoryRepository{}, Auth: auth, Settings: service}).Handler()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/settings/connections/credentials", nil))
		if response.Code != 401 && response.Code != 403 {
			t.Fatal("credential status exposed to a non-administrator")
		}
	}
	configuration, revision := service.Current()
	configuration.Connections[0].Renewal = &config.TokenRenewal{Namespace: "reader-system", ServiceAccount: "reader"}
	configuration.Connections[0].Credential.Token = "e30." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"system:serviceaccount:reader-system:reader","marker":"private-reader-token"}`)) + ".signature"
	if _, err := service.SaveReplacing(context.Background(), revision, configuration, "admin", "Renewal enabled", []string{"cluster"}); err != nil {
		t.Fatal(err)
	}
	target := service.CredentialTargets()[0]
	state := target.State
	last := time.Now().Add(-2 * time.Hour).UTC()
	expires, next := last.Add(time.Hour), last.Add(20*time.Minute)
	state.State, state.ExpiryVerified = settings.RenewalHealthy, true
	state.ExpiresAt, state.NextAttemptAt, state.LastSuccessAt = &expires, &next, &last
	if err := service.UpdateCredential(context.Background(), target, state); err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Repository: &memoryRepository{}, Auth: viewerAuth{allowed: true, user: config.User{Subject: "admin", Admin: true}}, Settings: service}).Handler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/settings/connections/credentials", nil))
	if response.Code != 200 || strings.Contains(response.Body.String(), state.Credential.Token) || strings.Contains(response.Body.String(), "private-client-secret") || strings.Contains(response.Body.String(), "api.example") {
		t.Fatal("lifecycle projection exposed credential material")
	}
	var result contract.CredentialStatuses
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Credentials) != 1 || result.Credentials[0].State != "reconnectRequired" || result.Credentials[0].Mode != "automatic" || result.Credentials[0].ExpirySource != "tokenRequest" || result.Credentials[0].NextAttemptAt != nil {
		t.Fatal("expired server-issued token was not actionable")
	}
}
