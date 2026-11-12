package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

func TestManagedConnectionAPIGroupDefaultAndRoundTrip(t *testing.T) {
	t.Parallel()
	for _, group := range []config.APIGroup{"", config.APIGroupOpenBao, config.APIGroupKubeBao} {
		connection := config.Connection{ID: "connection", APIGroup: group, Access: config.Access{Mode: config.AccessInCluster}}
		view := connectionView(connection)
		if view.ApiGroup == nil || config.APIGroup(*view.ApiGroup) != connection.EffectiveAPIGroup() {
			t.Fatal("settings response omitted the effective API group")
		}
		decoded, err := connectionInput(view)
		if err != nil || decoded.EffectiveAPIGroup() != connection.EffectiveAPIGroup() {
			t.Fatal("managed connection lost its API group")
		}
	}
	legacy, err := connectionInput(contract.ManagedConnection{Authentication: contract.ManagedConnectionAuthenticationLocal})
	if err != nil || legacy.EffectiveAPIGroup() != config.APIGroupOpenBao {
		t.Fatal("an old API request changed its collection group")
	}
	invalid := contract.ManagedConnectionApiGroup("other.org")
	if _, err := connectionInput(contract.ManagedConnection{ApiGroup: &invalid}); err == nil {
		t.Fatal("unknown managed API group accepted")
	}
}

func TestAPIGroupCannotChangeThroughSettingsOrPreviews(t *testing.T) {
	t.Parallel()
	for _, group := range []config.APIGroup{config.APIGroupOpenBao, config.APIGroupKubeBao} {
		for _, operation := range []string{"save", "check", "rbac"} {
			t.Run(string(group)+"/"+operation, func(t *testing.T) {
				base := configurationService(t)
				initial, _ := base.Current()
				initial.Connections[0].APIGroup = group
				service, err := settings.Open(t.Context(), &configurationRepository{}, bytes.Repeat([]byte{2}, 32), &initial)
				if err != nil {
					t.Fatal(err)
				}
				handler := (&Server{
					Fleet: &memoryFleet{}, Auth: viewerAuth{user: config.User{Subject: "admin", Admin: true}, allowed: true},
					Settings: service, Origin: "https://console.example",
				}).Handler()
				configuration := configurationView(initial)
				if group == config.APIGroupKubeBao {
					// An older client omitting the group cannot reset native enrollment.
					configuration.Connections[0].ApiGroup = nil
				} else {
					native := contract.KubebaoOrg
					configuration.Connections[0].ApiGroup = &native
				}
				method, path := http.MethodPost, "/api/v1/settings/connections/"+operation
				var input any
				switch operation {
				case "save":
					method, path = http.MethodPut, "/api/v1/settings"
					input = contract.SettingsUpdate{Revision: 1, Configuration: configuration}
				case "check":
					input = contract.ConnectionCheck{Connection: configuration.Connections[0]}
				case "rbac":
					input = contract.ReaderAccessInput{Connection: configuration.Connections[0]}
				}
				body, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				request := httptest.NewRequest(method, path, bytes.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Origin", "https://console.example")
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "API group cannot change") {
					t.Fatalf("API-group rebind reached %s: %d %s", operation, response.Code, response.Body.String())
				}
				current, revision := service.Current()
				if revision != 1 || current.Connections[0].EffectiveAPIGroup() != group {
					t.Fatal("rejected rebind changed enrollment")
				}
			})
		}
	}
}

func TestNativeReaderInstructionsUseOnlySelectedAPI(t *testing.T) {
	t.Parallel()
	service := configurationService(t)
	native := contract.KubebaoOrg
	connection := contract.ManagedConnection{
		Id: "native", Name: "Native", ApiGroup: &native, Authentication: contract.ManagedConnectionAuthenticationLocal,
		Namespaces: []contract.ManagedNamespace{{Name: "team", Environment: "production"}},
	}
	body, err := json.Marshal(contract.ReaderAccessInput{Connection: connection})
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{
		Fleet: &memoryFleet{}, Auth: viewerAuth{user: config.User{Subject: "admin", Admin: true}, allowed: true},
		Settings: service, Origin: "https://console.example",
	}).Handler()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/settings/connections/rbac", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://console.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var result contract.ReaderAccessResult
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil ||
		!strings.Contains(result.Rbac, "kubebao.org") || strings.Contains(result.Rbac, "openbao.org") {
		t.Fatal("native RBAC broadened the selected API group")
	}
	if _, revision := service.Current(); revision != 1 {
		t.Fatal("reader instruction preview changed enrollment")
	}
}
