package settings

import (
	"context"
	"fmt"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

// invalidError is an administrator error. Its message is safe to show to administrators.
type invalidError struct{ message string }

func (e *invalidError) Error() string        { return e.message }
func (e *invalidError) Is(target error) bool { return target == ErrInvalid }

func invalid(format string, args ...any) error {
	return &invalidError{message: fmt.Sprintf(format, args...)}
}

// Edit is an administrator's complete configuration change. The identity provider and
// the issuer of access rules are not part of an edit; they change only through a
// verified sign-in or local recovery.
type Edit struct {
	Revision      int64
	Configuration config.Config
	// Servers holds the API server the administrator saw for each remote connection.
	// Changing it requires a replacement credential.
	Servers map[string]string
	// Credentials replaces the reader credentials of the named remote connections.
	Credentials map[string]config.Credential
}

// ApplyEdit saves an administrator's change. Remote connections keep their stored reader
// credential unless the edit replaces it.
func (s *Service) ApplyEdit(ctx context.Context, edit Edit, actor string) (int64, error) {
	s.commit.Lock()
	defer s.commit.Unlock()
	if edit.Revision != s.revision {
		return 0, ErrConflict
	}
	next := clone(edit.Configuration)
	// Edited users and groups belong to the active issuer.
	next.Identity = s.configuration.Identity
	next.AccessIssuer = s.configuration.AccessIssuer
	replacements := []string{}
	for i := range next.Connections {
		connection := &next.Connections[i]
		var replacement *config.Credential
		if credential, found := edit.Credentials[connection.ID]; found {
			replacement = &credential
			replacements = append(replacements, connection.ID)
		}
		resolved, err := resolveAccess(*connection, edit.Servers[connection.ID], replacement, s.storedCredential(connection.ID))
		if err != nil {
			return 0, err
		}
		*connection = resolved
	}
	if len(replacements) != len(edit.Credentials) {
		return 0, invalid("credential references an unknown connection")
	}
	return s.save(ctx, edit.Revision, next, actor, Summary(s.configuration, next), replacements)
}

// ResolveConnection applies the edit rules to one connection without saving it.
func (s *Service) ResolveConnection(
	connection config.Connection,
	server string,
	replacement *config.Credential,
) (config.Connection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return resolveAccess(connection, server, replacement, s.storedCredential(connection.ID))
}

func (s *Service) storedCredential(connectionID string) *config.Credential {
	stored, found := s.credentials[connectionID]
	if !found {
		return nil
	}
	credential := stored.State.Credential
	return &credential
}

// resolveAccess decides which reader credential an edited connection uses. A changed API
// server never inherits the credential issued for the previous one.
func resolveAccess(connection config.Connection, server string, replacement, stored *config.Credential) (config.Connection, error) {
	if connection.Access.Mode != config.AccessRemote {
		if replacement != nil {
			return connection, invalid("remote credentials require remote authentication")
		}
		connection.Access.Credential = nil
		return connection, nil
	}
	switch {
	case replacement != nil:
		credential := *replacement
		connection.Access.Credential = &credential
	case stored == nil:
		return connection, invalid("remote connection requires a reader credential")
	case server != stored.Server:
		return connection, invalid("supply complete credentials when changing the API server")
	default:
		connection.Access.Credential = stored
	}
	return connection, nil
}
