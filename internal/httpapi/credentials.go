package httpapi

import (
	"net/http"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

func (s *Server) credentialStatuses(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.administrator(w, r); !ok {
		return
	}
	now := time.Now().UTC()
	result := contract.CredentialStatuses{Credentials: []contract.CredentialStatus{}}
	for _, target := range s.Settings.CredentialTargets() {
		status := contract.CredentialStatus{ConnectionId: target.Connection.ID, Mode: "local", State: "ready", ExpirySource: "unknown"}
		if target.Connection.Credential != nil {
			status.Mode = "manual"
			expiry := credentialExpiryView(target.Connection.Credential)
			status.ExpiresAt = expiry.ExpiresAt
			status.ExpirySource = contract.CredentialStatusExpirySource(expiry.Source)
			if target.Connection.Renewal != nil {
				status.Mode = "automatic"
				state := target.State
				status.State = contract.CredentialStatusState(state.State)
				status.ExpiresAt = state.ExpiresAt
				status.LastAttemptAt = state.LastAttemptAt
				status.LastSuccessAt = state.LastSuccessAt
				status.NextAttemptAt = state.NextAttemptAt
				if state.ExpiryVerified {
					status.ExpirySource = "tokenRequest"
					if state.ExpiresAt != nil && !now.Before(*state.ExpiresAt) {
						status.State = contract.CredentialStatusState(settings.RenewalReconnectRequired)
						status.NextAttemptAt = nil
					}
				}
			}
		}
		if target.Connection.Disabled {
			status.State = "paused"
		}
		result.Credentials = append(result.Credentials, status)
	}
	jsonResponse(w, 200, result)
}
