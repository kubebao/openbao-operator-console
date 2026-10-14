package auth

import (
	"slices"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

// Scope removes every resource the viewer may not see, including counts that would
// reveal hidden namespaces and relationships to hidden deployments. The store applies the
// viewer's scopes to activity before its page limit.
func Scope(snapshot fleet.Snapshot, user config.User) fleet.Snapshot {
	visible := user.CanView
	result := fleet.Snapshot{
		GeneratedAt:   snapshot.GeneratedAt,
		Policy:        snapshot.Policy,
		Deployments:   []fleet.Deployment{},
		Connections:   []fleet.Connection{},
		Relationships: []fleet.Relationship{},
	}
	deployments := make(map[string]bool)
	for _, deployment := range snapshot.Deployments {
		if visible(deployment.ConnectionID, deployment.Namespace) {
			result.Deployments = append(result.Deployments, deployment)
			deployments[deployment.ID] = true
		}
	}
	for _, connection := range snapshot.Connections {
		inScope := func(namespace string) bool { return visible(connection.ID, namespace) }
		connection.Namespaces = slices.DeleteFunc(
			slices.Clone(connection.Namespaces),
			func(namespace string) bool { return !inScope(namespace) },
		)
		if len(connection.Namespaces) == 0 {
			continue
		}
		operators := []fleet.Operator{}
		for _, operator := range connection.Operators {
			operator.Namespaces = slices.DeleteFunc(
				slices.Clone(operator.Namespaces),
				func(namespace string) bool { return !inScope(namespace) },
			)
			if len(operator.Namespaces) > 0 {
				operators = append(operators, operator)
			}
		}
		connection.Operators = operators
		connection.Capabilities = slices.DeleteFunc(
			slices.Clone(connection.Capabilities),
			func(capability fleet.Capability) bool { return !inScope(capability.Namespace) },
		)
		result.Connections = append(result.Connections, connection)
	}
	for _, relationship := range snapshot.Relationships {
		if deployments[relationship.SourceID] && deployments[relationship.TargetID] {
			result.Relationships = append(result.Relationships, relationship)
		}
	}
	return result
}
