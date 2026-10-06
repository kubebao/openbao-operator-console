package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/dc-tec/openbao-operator-console/internal/auth"
	"github.com/dc-tec/openbao-operator-console/internal/collector"
	"github.com/dc-tec/openbao-operator-console/internal/config"
	"github.com/dc-tec/openbao-operator-console/internal/contract"
	"github.com/dc-tec/openbao-operator-console/internal/evidence"
	"github.com/dc-tec/openbao-operator-console/internal/settings"
)

func scopesView(scopes []config.Scope) []contract.Scope {
	result := []contract.Scope{}
	for _, scope := range scopes {
		result = append(result, contract.Scope{ConnectionId: scope.ConnectionID, Namespaces: append([]string{}, scope.Namespaces...)})
	}
	return result
}
func scopesInput(scopes []contract.Scope) []config.Scope {
	result := []config.Scope{}
	for _, scope := range scopes {
		result = append(result, config.Scope{ConnectionID: scope.ConnectionId, Namespaces: append([]string{}, scope.Namespaces...)})
	}
	return result
}
func connectionView(c config.Connection) contract.ManagedConnection {
	result := contract.ManagedConnection{Id: c.ID, Name: c.Name, Region: c.Region, Disabled: c.Disabled, Authentication: "local", Namespaces: []contract.ManagedNamespace{}, Operators: []contract.ManagedOperator{}}
	if c.Credential != nil {
		result.Authentication = "remote"
		result.Server = c.Credential.Server
		result.CredentialConfigured = true
	}
	if c.Renewal != nil {
		result.Renewal = &contract.TokenRenewal{Namespace: c.Renewal.Namespace, ServiceAccount: c.Renewal.ServiceAccount}
	}
	for _, ns := range c.Namespaces {
		result.Namespaces = append(result.Namespaces, contract.ManagedNamespace{Name: ns.Name, Team: ns.Team, Environment: contract.ManagedNamespaceEnvironment(ns.Environment), Location: ns.Location})
	}
	for _, op := range c.Operators {
		result.Operators = append(result.Operators, contract.ManagedOperator{Id: op.ID, Namespace: op.Namespace, Deployment: op.Deployment, Container: op.Container, Namespaces: append([]string{}, op.Namespaces...)})
	}
	return result
}
func connectionInput(c contract.ManagedConnection, old *config.Connection) (config.Connection, error) {
	result := config.Connection{ID: c.Id, Name: c.Name, Region: c.Region, Disabled: c.Disabled, Namespaces: []config.Namespace{}, Operators: []config.Operator{}}
	switch c.Authentication {
	case "local":
	case "remote":
		result.Remote = true
		if old != nil {
			result.Credential = old.Credential
		}
	default:
		return result, fmt.Errorf("choose local or remote authentication")
	}
	if c.Renewal != nil {
		result.Renewal = &config.TokenRenewal{Namespace: c.Renewal.Namespace, ServiceAccount: c.Renewal.ServiceAccount}
	}
	for _, ns := range c.Namespaces {
		result.Namespaces = append(result.Namespaces, config.Namespace{Name: ns.Name, Team: ns.Team, Environment: string(ns.Environment), Location: ns.Location})
	}
	for _, op := range c.Operators {
		result.Operators = append(result.Operators, config.Operator{ID: op.Id, Namespace: op.Namespace, Deployment: op.Deployment, Container: op.Container, Namespaces: append([]string{}, op.Namespaces...)})
	}
	return result, nil
}
func resourceView(r config.ResourceRef) contract.ResourceRef {
	return contract.ResourceRef{ConnectionId: r.ConnectionID, Namespace: r.Namespace, Name: r.Name, Uid: r.UID}
}
func resourceInput(r contract.ResourceRef) config.ResourceRef {
	return config.ResourceRef{ConnectionID: r.ConnectionId, Namespace: r.Namespace, Name: r.Name, UID: r.Uid}
}
func configurationView(c config.Config) contract.Configuration {
	result := contract.Configuration{Connections: []contract.ManagedConnection{}, Users: []contract.ManagedUser{}, Groups: []contract.GroupBinding{}, Relationships: []contract.ManagedRelationship{}, MaxBackupAgeHours: c.MaxBackupAgeHours, StartupGraceMinutes: c.StartupGraceMinutes}
	for _, connection := range c.Connections {
		result.Connections = append(result.Connections, connectionView(connection))
	}
	for _, user := range c.Users {
		result.Users = append(result.Users, contract.ManagedUser{Subject: user.Subject, Name: user.Name, Admin: user.Admin, Scopes: scopesView(user.Scopes)})
	}
	for _, group := range c.Groups {
		result.Groups = append(result.Groups, contract.GroupBinding{Name: group.Name, Admin: group.Admin, Scopes: scopesView(group.Scopes)})
	}
	for _, route := range c.Relationships {
		result.Relationships = append(result.Relationships, contract.ManagedRelationship{Id: route.ID, Source: resourceView(route.Source), Target: resourceView(route.Target), Kind: contract.ManagedRelationshipKind(route.Kind), Repository: route.Repository})
	}
	return result
}
func credentialInput(credential *contract.ClusterCredential, kubeconfig, contextName *string) (*config.Credential, error) {
	if credential != nil && kubeconfig != nil {
		return nil, fmt.Errorf("supply structured credentials or a kubeconfig")
	}
	if credential != nil {
		result := &config.Credential{Server: credential.Server, CA: credential.Ca, Token: credential.Token, Certificate: credential.Certificate, Key: credential.Key}
		return result, result.Validate()
	}
	if kubeconfig != nil {
		selected := ""
		if contextName != nil {
			selected = *contextName
		}
		result, err := config.ImportKubeconfig([]byte(*kubeconfig), selected)
		return &result, err
	}
	return nil, nil
}
func configurationInput(input contract.SettingsUpdate, previous config.Config) (config.Config, error) {
	result := config.Config{Connections: []config.Connection{}, Users: []config.User{}, Groups: []config.Group{}, Relationships: []config.Relationship{}, MaxBackupAgeHours: input.Configuration.MaxBackupAgeHours, StartupGraceMinutes: input.Configuration.StartupGraceMinutes, Identity: previous.Identity}
	old := map[string]config.Connection{}
	for _, connection := range previous.Connections {
		old[connection.ID] = connection
	}
	credentials := map[string]contract.CredentialUpdate{}
	for _, update := range input.Credentials {
		if _, found := credentials[update.ConnectionId]; found {
			return result, fmt.Errorf("duplicate credential update")
		}
		credentials[update.ConnectionId] = update
	}
	for _, connection := range input.Configuration.Connections {
		if existing, ok := old[connection.Id]; ok && connection.Authentication == "remote" && existing.Credential != nil && connection.Server != existing.Credential.Server {
			if _, updated := credentials[connection.Id]; !updated {
				return result, fmt.Errorf("supply complete credentials when changing the API server")
			}
		}
		existing := old[connection.Id]
		next, err := connectionInput(connection, &existing)
		if err != nil {
			return result, err
		}
		if update, found := credentials[connection.Id]; found {
			if connection.Authentication != "remote" {
				return result, fmt.Errorf("remote credentials require remote authentication")
			}
			next.Credential, err = credentialInput(update.Credential, update.Kubeconfig, update.Context)
			if err != nil {
				return result, err
			}
			delete(credentials, connection.Id)
		}
		if connection.Authentication == "remote" && next.Credential == nil {
			return result, fmt.Errorf("remote connection requires reader credentials")
		}
		result.Connections = append(result.Connections, next)
	}
	if len(credentials) > 0 {
		return result, fmt.Errorf("credential references an unknown connection")
	}
	for _, user := range input.Configuration.Users {
		result.Users = append(result.Users, config.User{Subject: user.Subject, Name: user.Name, Admin: user.Admin, Scopes: scopesInput(user.Scopes)})
	}
	for _, group := range input.Configuration.Groups {
		result.Groups = append(result.Groups, config.Group{Name: group.Name, Admin: group.Admin, Scopes: scopesInput(group.Scopes)})
	}
	for _, route := range input.Configuration.Relationships {
		result.Relationships = append(result.Relationships, config.Relationship{ID: route.Id, Source: resourceInput(route.Source), Target: resourceInput(route.Target), Kind: string(route.Kind), Repository: route.Repository})
	}
	return result, result.Validate()
}
func (s *Server) settingsView(ctx context.Context) (contract.Settings, error) {
	configuration, revision := s.Settings.Current()
	result := contract.Settings{Revision: revision, Configuration: configurationView(configuration), Changes: []contract.SettingsChange{}}
	if identity := configuration.Identity; identity != nil {
		result.Identity = contract.IdentitySummary{Issuer: identity.Issuer, ClientId: identity.ClientID, Ca: identity.CA, GroupsClaim: identity.GroupsClaim, SecretConfigured: identity.ClientSecret != ""}
	}
	changes, err := s.Settings.History(ctx)
	if err != nil {
		return result, err
	}
	for _, change := range changes {
		result.Changes = append(result.Changes, contract.SettingsChange{Revision: change.Revision, Actor: change.Actor, Summary: change.Summary, At: change.At})
	}
	return result, nil
}
func (s *Server) administrator(w http.ResponseWriter, r *http.Request) (config.User, bool) {
	user, allowed := s.Auth.Viewer(r)
	if !allowed {
		problem(w, 401, "unauthenticated", "Sign in to manage settings.")
		return user, false
	}
	if !user.Admin {
		problem(w, 403, "forbidden", "Console administrator access is required.")
		return user, false
	}
	if s.Settings == nil {
		problem(w, 503, "unavailable", "Settings are unavailable.")
		return user, false
	}
	if r.Method != "GET" && r.Header.Get("Origin") != s.Origin {
		problem(w, 403, "forbidden", "Invalid request origin.")
		return user, false
	}
	return user, true
}
func decodeBody[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	var result T
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return result, fmt.Errorf("use application/json")
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("request body is invalid or exceeds 1 MiB")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return result, fmt.Errorf("request must contain one JSON object")
	}
	return result, nil
}

func (s *Server) readerAccess(connection config.Connection, authentication contract.ManagedConnectionAuthentication) (contract.ReaderAccessResult, error) {
	readerName, readerNamespace := "console-reader", "console-system"
	if authentication == "local" && s.ReaderName != "" && s.ReaderNamespace != "" {
		readerName, readerNamespace = s.ReaderName, s.ReaderNamespace
	}
	if connection.Renewal != nil {
		readerName, readerNamespace = connection.Renewal.ServiceAccount, connection.Renewal.Namespace
	}
	var manifests bytes.Buffer
	if err := config.WriteRBAC(&manifests, connection, readerName, readerNamespace); err != nil {
		return contract.ReaderAccessResult{}, err
	}
	return contract.ReaderAccessResult{Rbac: manifests.String(), ServiceAccount: readerName, ServiceAccountNamespace: readerNamespace}, nil
}

func credentialExpiryView(credential *config.Credential) contract.CredentialExpiry {
	expiry := credential.Expiry()
	source := contract.CredentialExpirySourceUnknown
	switch expiry.Source {
	case config.ExpiryCertificate:
		source = contract.CredentialExpirySourceCertificate
	case config.ExpiryDeclaredToken:
		source = contract.CredentialExpirySourceDeclaredToken
	}
	return contract.CredentialExpiry{ExpiresAt: expiry.ExpiresAt, Source: source}
}

func (s *Server) settingsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/settings/connections/credentials", s.credentialStatuses)
	mux.HandleFunc("GET /api/v1/settings", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.administrator(w, r); !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		result, err := s.settingsView(ctx)
		if err != nil {
			problem(w, 503, "unavailable", "Settings could not be read.")
			return
		}
		jsonResponse(w, 200, result)
	})
	mux.HandleFunc("PUT /api/v1/settings", func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.administrator(w, r)
		if !ok {
			return
		}
		input, err := decodeBody[contract.SettingsUpdate](w, r)
		if err != nil {
			problem(w, 400, "invalid", err.Error())
			return
		}
		previous, revision := s.Settings.Current()
		if input.Revision != revision {
			problem(w, 409, "conflict", "Settings changed. Reload before saving.")
			return
		}
		next, err := configurationInput(input, previous)
		if err != nil {
			problem(w, 400, "invalid", err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		replacements := []string{}
		for _, update := range input.Credentials {
			replacements = append(replacements, update.ConnectionId)
		}
		if _, err := s.Settings.SaveReplacing(ctx, input.Revision, next, user.Subject, settings.Summary(previous, next), replacements); err != nil {
			if errors.Is(err, settings.ErrConflict) {
				problem(w, 409, "conflict", "Settings changed. Reload before saving.")
			} else if errors.Is(err, settings.ErrInvalid) {
				problem(w, 400, "invalid", err.Error())
			} else {
				problem(w, 503, "unavailable", "Settings could not be saved. Retry after the database is available.")
			}
			return
		}
		result, err := s.settingsView(ctx)
		if err != nil {
			problem(w, 503, "unavailable", "Settings saved; reload to read the result.")
			return
		}
		jsonResponse(w, 200, result)
	})
	mux.HandleFunc("POST /api/v1/settings/connections/rbac", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.administrator(w, r); !ok {
			return
		}
		input, err := decodeBody[contract.ReaderAccessInput](w, r)
		if err != nil {
			problem(w, 400, "invalid", err.Error())
			return
		}
		connection, err := connectionInput(input.Connection, nil)
		if err != nil {
			problem(w, 400, "invalid", err.Error())
			return
		}
		access, err := s.readerAccess(connection, input.Connection.Authentication)
		if err != nil {
			problem(w, 400, "invalid", err.Error())
			return
		}
		jsonResponse(w, 200, access)
	})
	mux.HandleFunc("POST /api/v1/settings/connections/check", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.administrator(w, r); !ok {
			return
		}
		input, err := decodeBody[contract.ConnectionCheck](w, r)
		if err != nil {
			problem(w, 400, "invalid", err.Error())
			return
		}
		current, _ := s.Settings.Current()
		var old *config.Connection
		for _, connection := range current.Connections {
			if connection.ID == input.Connection.Id {
				copy := connection
				old = &copy
				break
			}
		}
		connection, err := connectionInput(input.Connection, old)
		if err == nil && input.Connection.Authentication == "remote" && connection.Credential != nil && input.Credential == nil && input.Kubeconfig == nil && input.Connection.Server != connection.Credential.Server {
			err = fmt.Errorf("supply complete credentials when changing the API server")
		}
		if err == nil && input.Connection.Authentication == "local" && (input.Credential != nil || input.Kubeconfig != nil) {
			err = fmt.Errorf("remote credentials require remote authentication")
		}
		if err == nil && (input.Credential != nil || input.Kubeconfig != nil) {
			connection.Credential, err = credentialInput(input.Credential, input.Kubeconfig, input.Context)
		}
		if err == nil && input.Connection.Authentication == "remote" && connection.Credential == nil {
			err = fmt.Errorf("supply a remote reader credential")
		}
		checkConfig := config.Config{Connections: []config.Connection{connection}, MaxBackupAgeHours: current.MaxBackupAgeHours, StartupGraceMinutes: current.StartupGraceMinutes}
		if err == nil {
			err = checkConfig.Validate()
		}
		if err != nil {
			problem(w, 400, "invalid", err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		reader := collector.Unconfigured(connection, evidence.Policy{MaxBackupAge: time.Duration(current.MaxBackupAgeHours) * time.Hour, StartupGrace: time.Duration(current.StartupGraceMinutes) * time.Minute})
		batch, diagnostic := reader.Check(ctx, time.Now().UTC())
		access, err := s.readerAccess(connection, input.Connection.Authentication)
		if err != nil {
			problem(w, 400, "invalid", "Reader RBAC could not be generated.")
			return
		}
		deployments := batch.Deployments
		if deployments == nil {
			deployments = []contract.Deployment{}
		}
		jsonResponse(w, 200, contract.ConnectionCheckResult{Connection: batch.Connection, Rbac: access.Rbac, Deployments: deployments,
			Diagnostics:      contract.ConnectionDiagnostic{State: diagnostic.State, Message: diagnostic.Message},
			CredentialExpiry: credentialExpiryView(connection.Credential)})
	})
	mux.HandleFunc("POST /api/v1/settings/identity/trial", s.identityTrial)
}
func (s *Server) identityTrial(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.administrator(w, r)
	if !ok {
		return
	}
	authentication, ok := s.Auth.(*auth.Authenticator)
	if !ok {
		problem(w, 503, "unavailable", "Identity setup is unavailable.")
		return
	}
	input, err := decodeBody[contract.IdentityTrial](w, r)
	if err != nil {
		problem(w, 400, "invalid", err.Error())
		return
	}
	current, revision := s.Settings.Current()
	if input.Revision != revision {
		problem(w, 409, "conflict", "Settings changed. Reload before testing.")
		return
	}
	secret := ""
	if current.Identity != nil && current.Identity.Issuer == input.Identity.Issuer && current.Identity.ClientID == input.Identity.ClientId {
		secret = current.Identity.ClientSecret
	}
	if input.Identity.ClientSecret != nil {
		secret = *input.Identity.ClientSecret
	}
	candidateIdentity := config.Identity{Issuer: input.Identity.Issuer, ClientID: input.Identity.ClientId, ClientSecret: secret, CA: input.Identity.Ca, GroupsClaim: input.Identity.GroupsClaim}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	providerContext, err := auth.ProviderContextPEM(ctx, []byte(candidateIdentity.CA))
	if err != nil {
		problem(w, 400, "invalid", "Check the identity provider CA certificate.")
		return
	}
	candidate, err := auth.NewOIDC(providerContext, candidateIdentity.Issuer, candidateIdentity.ClientID, candidateIdentity.ClientSecret, s.Origin+"/auth/callback", nil)
	if err != nil {
		problem(w, 400, "invalid", "Identity discovery failed. Check the issuer, HTTPS trust, and client settings.")
		return
	}
	candidate.SetGroupsClaim(candidateIdentity.GroupsClaim)
	location, err := authentication.BeginTrial(w, candidate, func(ctx context.Context, subject string, groups []string) error {
		if candidateUser, allowed := s.Settings.Resolve(ctx, subject, groups); !allowed || !candidateUser.Admin {
			return fmt.Errorf("candidate login is not an enrolled administrator")
		}
		latest, version := s.Settings.Current()
		if version != revision {
			return settings.ErrConflict
		}
		if original, allowed := s.Auth.Viewer(r); !allowed || !original.Admin {
			return fmt.Errorf("initiating administrator is no longer authorized")
		}
		latest.Identity = &candidateIdentity
		if _, err := s.Settings.Save(ctx, revision, latest, actor.Subject, "Identity provider verified and activated"); err != nil {
			return err
		}
		authentication.ReplaceProvider(candidate)
		return nil
	})
	if err != nil {
		problem(w, 503, "unavailable", "Cannot begin identity verification.")
		return
	}
	jsonResponse(w, 200, contract.LoginRedirect{Url: location})
}

func filterEnrollment(snapshot contract.Snapshot, c config.Config) contract.Snapshot {
	enrolled := map[string]config.Connection{}
	for _, connection := range c.Connections {
		enrolled[connection.ID] = connection
	}
	connections := []contract.Connection{}
	for _, connection := range snapshot.Connections {
		if current, ok := enrolled[connection.Id]; ok {
			connection.Name = current.Name
			connection.Region = current.Region
			connection.Namespaces = []string{}
			for _, ns := range current.Namespaces {
				connection.Namespaces = append(connection.Namespaces, ns.Name)
			}
			operators := []contract.Operator{}
			for _, declared := range current.Operators {
				observation := contract.Operator{Id: declared.ID, Namespace: declared.Namespace, Name: declared.Deployment, Container: declared.Container, Collection: "pending", Health: "unknown", LastAttemptAt: connection.LastAttemptAt}
				for _, previous := range connection.Operators {
					if previous.Id == declared.ID && previous.Namespace == declared.Namespace && previous.Name == declared.Deployment && previous.Container == declared.Container {
						observation = previous
						break
					}
				}
				observation.Namespaces = append([]string{}, declared.Namespaces...)
				if current.Disabled {
					observation.Collection = "stale"
				}
				operators = append(operators, observation)
			}
			connection.Operators = operators
			capabilities := []contract.ApiCapability{}
			for _, capability := range connection.Capabilities {
				for _, namespace := range current.Namespaces {
					if capability.Namespace == namespace.Name {
						capabilities = append(capabilities, capability)
						break
					}
				}
			}
			connection.Capabilities = capabilities
			if current.Disabled {
				connection.Collection = "stale"
			}
			connections = append(connections, connection)
		}
	}
	deployments := []contract.Deployment{}
	for _, deployment := range snapshot.Deployments {
		if connection, ok := enrolled[deployment.ConnectionId]; ok {
			for _, ns := range connection.Namespaces {
				if ns.Name == deployment.Namespace {
					deployment.Team = ns.Team
					deployment.Environment = contract.DeploymentEnvironment(ns.Environment)
					deployment.Location = ns.Location
					if connection.Disabled {
						evidence.CollectionFailure(&deployment, "stale", deployment.Evidence.LastAttemptAt)
					}
					deployments = append(deployments, deployment)
					break
				}
			}
		}
	}
	snapshot.Connections = connections
	snapshot.Deployments = deployments
	return snapshot
}
