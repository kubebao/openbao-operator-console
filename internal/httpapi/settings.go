package httpapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/auth"
	"github.com/kubebao/openbao-operator-console/internal/collector"
	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
	"github.com/kubebao/openbao-operator-console/internal/settings"
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
	result := contract.ManagedConnection{
		Id:             c.ID,
		Name:           c.Name,
		Region:         c.Region,
		Disabled:       c.Disabled,
		Authentication: "local",
		Namespaces:     []contract.ManagedNamespace{},
		Operators:      []contract.ManagedOperator{},
	}
	if c.Access.Mode == config.AccessRemote {
		result.Authentication = "remote"
		if c.Access.Credential != nil {
			result.Server = c.Access.Credential.Server
			result.CredentialConfigured = true
		}
	}
	if c.Renewal != nil {
		result.Renewal = &contract.TokenRenewal{Namespace: c.Renewal.Namespace, ServiceAccount: c.Renewal.ServiceAccount}
	}
	for _, ns := range c.Namespaces {
		result.Namespaces = append(
			result.Namespaces,
			contract.ManagedNamespace{
				Name:        ns.Name,
				Team:        ns.Team,
				Environment: contract.ManagedNamespaceEnvironment(ns.Environment),
				Location:    ns.Location,
			},
		)
	}
	for _, op := range c.Operators {
		result.Operators = append(
			result.Operators,
			contract.ManagedOperator{
				Id:         op.ID,
				Namespace:  op.Namespace,
				Deployment: op.Deployment,
				Container:  op.Container,
				Namespaces: append([]string{}, op.Namespaces...),
			},
		)
	}
	return result
}

func connectionInput(c contract.ManagedConnection) (config.Connection, error) {
	result := config.Connection{
		ID:         c.Id,
		Name:       c.Name,
		Region:     c.Region,
		Disabled:   c.Disabled,
		Namespaces: []config.Namespace{},
		Operators:  []config.Operator{},
	}
	switch c.Authentication {
	case contract.ManagedConnectionAuthenticationLocal:
		result.Access = config.Access{Mode: config.AccessInCluster}
	case contract.ManagedConnectionAuthenticationRemote:
		result.Access = config.Access{Mode: config.AccessRemote}
	default:
		return result, errors.New("choose local or remote authentication")
	}
	if c.Renewal != nil {
		result.Renewal = &config.TokenRenewal{Namespace: c.Renewal.Namespace, ServiceAccount: c.Renewal.ServiceAccount}
	}
	for _, ns := range c.Namespaces {
		result.Namespaces = append(
			result.Namespaces,
			config.Namespace{Name: ns.Name, Team: ns.Team, Environment: fleet.Environment(ns.Environment), Location: ns.Location},
		)
	}
	for _, op := range c.Operators {
		result.Operators = append(
			result.Operators,
			config.Operator{
				ID:         op.Id,
				Namespace:  op.Namespace,
				Deployment: op.Deployment,
				Container:  op.Container,
				Namespaces: append([]string{}, op.Namespaces...),
			},
		)
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
	result := contract.Configuration{
		Connections:         []contract.ManagedConnection{},
		Users:               []contract.ManagedUser{},
		Groups:              []contract.GroupBinding{},
		Relationships:       []contract.ManagedRelationship{},
		MaxBackupAgeHours:   c.MaxBackupAgeHours,
		StartupGraceMinutes: c.StartupGraceMinutes,
	}
	for _, connection := range c.Connections {
		result.Connections = append(result.Connections, connectionView(connection))
	}
	for _, user := range c.Users {
		result.Users = append(
			result.Users,
			contract.ManagedUser{Subject: user.Subject, Name: user.Name, Admin: user.Admin, Scopes: scopesView(user.Scopes)},
		)
	}
	for _, group := range c.Groups {
		result.Groups = append(result.Groups, contract.GroupBinding{Name: group.Name, Admin: group.Admin, Scopes: scopesView(group.Scopes)})
	}
	for _, route := range c.Relationships {
		result.Relationships = append(
			result.Relationships,
			contract.ManagedRelationship{
				Id:         route.ID,
				Source:     resourceView(route.Source),
				Target:     resourceView(route.Target),
				Kind:       contract.ManagedRelationshipKind(route.Kind),
				Repository: route.Repository,
			},
		)
	}
	return result
}

// credentialInput accepts either structured credentials or a kubeconfig, never both.
func credentialInput(credential *contract.ClusterCredential, kubeconfig, contextName *string) (*config.Credential, error) {
	switch {
	case credential != nil && kubeconfig != nil:
		return nil, errors.New("supply structured credentials or a kubeconfig")
	case credential != nil:
		result := &config.Credential{
			Server:      credential.Server,
			CA:          credential.Ca,
			Token:       credential.Token,
			Certificate: credential.Certificate,
			Key:         credential.Key,
		}
		return result, result.Validate()
	case kubeconfig != nil:
		selected := ""
		if contextName != nil {
			selected = *contextName
		}
		result, err := config.ImportKubeconfig([]byte(*kubeconfig), selected)
		return &result, err
	default:
		return nil, nil
	}
}

// editInput maps a settings update onto a settings edit. The settings service applies the
// credential rules.
func editInput(input contract.SettingsUpdate) (settings.Edit, error) {
	edit := settings.Edit{
		Revision: input.Revision,
		Configuration: config.Config{
			Connections:         []config.Connection{},
			Users:               []config.User{},
			Groups:              []config.Group{},
			Relationships:       []config.Relationship{},
			MaxBackupAgeHours:   input.Configuration.MaxBackupAgeHours,
			StartupGraceMinutes: input.Configuration.StartupGraceMinutes,
		},
		Servers:     map[string]string{},
		Credentials: map[string]config.Credential{},
	}
	for _, managed := range input.Configuration.Connections {
		connection, err := connectionInput(managed)
		if err != nil {
			return edit, err
		}
		edit.Configuration.Connections = append(edit.Configuration.Connections, connection)
		if managed.Authentication == contract.ManagedConnectionAuthenticationRemote {
			edit.Servers[managed.Id] = managed.Server
		}
	}
	for _, update := range input.Credentials {
		if _, found := edit.Credentials[update.ConnectionId]; found {
			return edit, errors.New("duplicate credential update")
		}
		credential, err := credentialInput(update.Credential, update.Kubeconfig, update.Context)
		if err != nil {
			return edit, err
		}
		if credential == nil {
			return edit, errors.New("credential update requires structured credentials or a kubeconfig")
		}
		edit.Credentials[update.ConnectionId] = *credential
	}
	for _, user := range input.Configuration.Users {
		edit.Configuration.Users = append(
			edit.Configuration.Users,
			config.User{Subject: user.Subject, Name: user.Name, Admin: user.Admin, Scopes: scopesInput(user.Scopes)},
		)
	}
	for _, group := range input.Configuration.Groups {
		edit.Configuration.Groups = append(
			edit.Configuration.Groups,
			config.Group{Name: group.Name, Admin: group.Admin, Scopes: scopesInput(group.Scopes)},
		)
	}
	for _, route := range input.Configuration.Relationships {
		edit.Configuration.Relationships = append(
			edit.Configuration.Relationships,
			config.Relationship{
				ID:         route.Id,
				Source:     resourceInput(route.Source),
				Target:     resourceInput(route.Target),
				Kind:       fleet.RelationshipKind(route.Kind),
				Repository: route.Repository,
			},
		)
	}
	return edit, nil
}

func (s *Server) settingsView(ctx context.Context) (contract.Settings, error) {
	configuration, revision := s.Settings.Current()
	result := contract.Settings{Revision: revision, Configuration: configurationView(configuration), Changes: []contract.SettingsChange{}}
	if identity := configuration.Identity; identity != nil {
		result.Identity = contract.IdentitySummary{
			Issuer:           identity.Issuer,
			ClientId:         identity.ClientID,
			Ca:               identity.CA,
			GroupsClaim:      identity.GroupsClaim,
			SecretConfigured: identity.ClientSecret != "",
		}
	}
	changes, err := s.Settings.History(ctx)
	if err != nil {
		return result, err
	}
	for _, change := range changes {
		result.Changes = append(
			result.Changes,
			contract.SettingsChange{Revision: change.Revision, Actor: change.Actor, Summary: change.Summary, At: change.At},
		)
	}
	return result, nil
}

func (s *Server) readerAccess(
	connection config.Connection,
	authentication contract.ManagedConnectionAuthentication,
) (contract.ReaderAccessResult, error) {
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
	mux.HandleFunc("GET /api/v1/settings", s.admin(s.readSettings))
	mux.HandleFunc("PUT /api/v1/settings", s.admin(s.saveSettings))
	mux.HandleFunc("GET /api/v1/settings/connections/credentials", s.admin(s.credentialStatuses))
	mux.HandleFunc("POST /api/v1/settings/connections/rbac", s.admin(s.readerRBAC))
	mux.HandleFunc("POST /api/v1/settings/connections/check", s.admin(s.checkConnection))
	mux.HandleFunc("POST /api/v1/settings/identity/trial", s.admin(s.identityTrial))
}

func (s *Server) readSettings(w http.ResponseWriter, r *http.Request, _ config.User) error {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	result, err := s.settingsView(ctx)
	if err != nil {
		return unavailable(err, "Settings could not be read.")
	}
	jsonResponse(w, http.StatusOK, result)
	return nil
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request, actor config.User) error {
	input, err := decodeBody[contract.SettingsUpdate](w, r)
	if err != nil {
		return err
	}
	edit, err := editInput(input)
	if err != nil {
		return invalidRequest(err)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if _, err := s.Settings.ApplyEdit(ctx, edit, actor.Subject); err != nil {
		return unavailable(err, "Settings could not be saved. Retry after the database is available.")
	}
	result, err := s.settingsView(ctx)
	if err != nil {
		return unavailable(err, "Settings saved; reload to read the result.")
	}
	jsonResponse(w, http.StatusOK, result)
	return nil
}

func (s *Server) readerRBAC(w http.ResponseWriter, r *http.Request, _ config.User) error {
	input, err := decodeBody[contract.ReaderAccessInput](w, r)
	if err != nil {
		return err
	}
	connection, err := connectionInput(input.Connection)
	if err != nil {
		return invalidRequest(err)
	}
	access, err := s.readerAccess(connection, input.Connection.Authentication)
	if err != nil {
		return invalidRequest(err)
	}
	jsonResponse(w, http.StatusOK, access)
	return nil
}

// checkConnection previews a connection under the saved policy without persisting it.
func (s *Server) checkConnection(w http.ResponseWriter, r *http.Request, _ config.User) error {
	input, err := decodeBody[contract.ConnectionCheck](w, r)
	if err != nil {
		return err
	}
	connection, err := connectionInput(input.Connection)
	if err != nil {
		return invalidRequest(err)
	}
	replacement, err := credentialInput(input.Credential, input.Kubeconfig, input.Context)
	if err != nil {
		return invalidRequest(err)
	}
	connection, err = s.Settings.ResolveConnection(connection, input.Connection.Server, replacement)
	if err != nil {
		return err
	}
	current, _ := s.Settings.Current()
	preview := config.Config{
		Connections:         []config.Connection{connection},
		MaxBackupAgeHours:   current.MaxBackupAgeHours,
		StartupGraceMinutes: current.StartupGraceMinutes,
	}
	if err := preview.Validate(); err != nil {
		return invalidRequest(err)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	batch, diagnostic := collector.Unconfigured(connection, current.Policy()).Check(ctx, time.Now().UTC())
	access, err := s.readerAccess(connection, input.Connection.Authentication)
	if err != nil {
		return badRequest("Reader RBAC could not be generated.")
	}
	jsonResponse(w, http.StatusOK, contract.ConnectionCheckResult{
		Connection:  connectionContract(batch.Connection),
		Rbac:        access.Rbac,
		Deployments: mapSlice(batch.Deployments, deploymentContract),
		Diagnostics: contract.ConnectionDiagnostic{
			State:   contract.ConnectionDiagnosticState(diagnostic.State),
			Message: diagnostic.Message,
		},
		CredentialExpiry: credentialExpiryView(connection.Access.Credential),
	})
	return nil
}

// identityTrial activates a candidate identity provider only after an enrolled
// administrator signs in through it.
func (s *Server) identityTrial(w http.ResponseWriter, r *http.Request, actor config.User) error {
	authentication, ok := s.Auth.(*auth.Authenticator)
	if !ok {
		return retryLater("Identity setup is unavailable.")
	}
	input, err := decodeBody[contract.IdentityTrial](w, r)
	if err != nil {
		return err
	}
	current, revision := s.Settings.Current()
	if input.Revision != revision {
		return &problemError{status: http.StatusConflict, code: "conflict", message: "Settings changed. Reload before testing."}
	}
	secret := ""
	if current.Identity != nil && current.Identity.Issuer == input.Identity.Issuer && current.Identity.ClientID == input.Identity.ClientId {
		secret = current.Identity.ClientSecret
	}
	if input.Identity.ClientSecret != nil {
		secret = *input.Identity.ClientSecret
	}
	candidateIdentity := config.Identity{
		Issuer:       input.Identity.Issuer,
		ClientID:     input.Identity.ClientId,
		ClientSecret: secret,
		CA:           input.Identity.Ca,
		GroupsClaim:  input.Identity.GroupsClaim,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	providerContext, err := auth.ProviderContextPEM(ctx, []byte(candidateIdentity.CA))
	if err != nil {
		return badRequest("Check the identity provider CA certificate.")
	}
	candidate, err := auth.NewOIDC(
		providerContext,
		candidateIdentity.Issuer,
		candidateIdentity.ClientID,
		candidateIdentity.ClientSecret,
		s.Origin+"/auth/callback",
		nil,
	)
	if err != nil {
		return badRequest("Identity discovery failed. Check the issuer, HTTPS trust, and client settings.")
	}
	candidate.SetGroupsClaim(candidateIdentity.GroupsClaim)
	location, err := authentication.BeginTrial(w, candidate, func(ctx context.Context, subject string, groups []string) error {
		if candidateUser, allowed := s.Settings.Resolve(ctx, subject, groups); !allowed || !candidateUser.Admin {
			return errors.New("candidate login is not an enrolled administrator")
		}
		latest, version := s.Settings.Current()
		if version != revision {
			return settings.ErrConflict
		}
		if original, allowed := s.Auth.Viewer(r); !allowed || !original.Admin {
			return errors.New("initiating administrator is no longer authorized")
		}
		latest.Identity = &candidateIdentity
		if _, err := s.Settings.Save(ctx, revision, latest, actor.Subject, "Identity provider verified and activated"); err != nil {
			return err
		}
		authentication.ReplaceProvider(candidate)
		return nil
	})
	if err != nil {
		return retryLater("Cannot begin identity verification.")
	}
	jsonResponse(w, http.StatusOK, contract.LoginRedirect{Url: location})
	return nil
}
