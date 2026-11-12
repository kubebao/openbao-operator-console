// Package view builds the fleet the console presents: retained observations reconciled
// with the current enrollment, with staleness evaluated at read time.
package view

import (
	"context"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/evidence"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

// StaleAfter is how long current evidence remains current without a new collection
// attempt. Each connection is collected every 30 seconds on its own schedule.
const StaleAfter = 2 * time.Minute

// Observations provides retained collection results.
type Observations interface {
	Snapshot(context.Context) (fleet.Snapshot, error)
	History(context.Context, string, fleet.HistoryQuery) (fleet.HistoryPage, error)
	Charts(context.Context, string, time.Time, time.Time) (fleet.ChartHistory, error)
	Activity(context.Context, []config.Scope, fleet.Page) (fleet.ActivityPage, error)
}

// Enrollment provides the current administrator configuration.
type Enrollment interface {
	Current() (config.Config, int64)
}

// Live reconciles retained observations with the current enrollment on every read.
type Live struct {
	Observations Observations
	Enrollment   Enrollment
}

func (l Live) Snapshot(ctx context.Context) (fleet.Snapshot, error) {
	stored, err := l.Observations.Snapshot(ctx)
	if err != nil {
		return fleet.Snapshot{}, err
	}
	enrollment, _ := l.Enrollment.Current()
	return Build(stored, enrollment, time.Now().UTC()), nil
}

func (l Live) History(ctx context.Context, id string, query fleet.HistoryQuery) (fleet.HistoryPage, error) {
	return l.Observations.History(ctx, id, query)
}

func (l Live) Charts(ctx context.Context, id string, start, end time.Time) (fleet.ChartHistory, error) {
	return l.Observations.Charts(ctx, id, start, end)
}

// Activity returns one page of retained events in the given scopes. Callers pass scopes
// already intersected with the current enrollment.
func (l Live) Activity(ctx context.Context, scopes []config.Scope, page fleet.Page) (fleet.ActivityPage, error) {
	return l.Observations.Activity(ctx, scopes, page)
}

// Build keeps only enrolled connections, namespaces, and controllers, applies current
// enrollment metadata, marks evidence without a recent attempt as stale, and resolves
// declared relationships between visible deployments.
func Build(stored fleet.Snapshot, enrollment config.Config, now time.Time) fleet.Snapshot {
	connections := buildConnections(stored.Connections, enrollment.Connections, now)
	deployments := buildDeployments(stored.Deployments, enrollment.Connections, connections, now)
	return fleet.Snapshot{
		GeneratedAt:   stored.GeneratedAt,
		Policy:        enrollment.Policy(),
		Connections:   connections,
		Deployments:   deployments,
		Relationships: declaredRelationships(enrollment.Relationships, deployments),
	}
}

func enrolledByID(connections []config.Connection) map[string]config.Connection {
	result := make(map[string]config.Connection, len(connections))
	for _, connection := range connections {
		result[connection.ID] = connection
	}
	return result
}

// Observed connections keep their stored order. Enrolled connections without an
// observation follow as pending.
func buildConnections(stored []fleet.Connection, enrolled []config.Connection, now time.Time) []fleet.Connection {
	byID := enrolledByID(enrolled)
	observed := map[string]bool{}
	result := []fleet.Connection{}
	for _, connection := range stored {
		current, ok := byID[connection.ID]
		if !ok {
			continue
		}
		observed[connection.ID] = true
		result = append(result, reconcileConnection(connection, current, now))
	}
	for _, connection := range enrolled {
		if !observed[connection.ID] {
			result = append(result, pendingConnection(connection, now))
		}
	}
	return result
}

func pendingConnection(enrolled config.Connection, now time.Time) fleet.Connection {
	return fleet.Connection{
		ID:            enrolled.ID,
		Name:          enrolled.Name,
		Region:        enrolled.Region,
		Collection:    fleet.ConnectionCollectionPending,
		LastAttemptAt: now,
		Namespaces:    namespaceNames(enrolled),
		Operators:     []fleet.Operator{},
		Capabilities:  []fleet.Capability{},
	}
}

func namespaceNames(connection config.Connection) []string {
	result := []string{}
	for _, namespace := range connection.Namespaces {
		result = append(result, namespace.Name)
	}
	return result
}

func enrolledNamespace(connection config.Connection, name string) (config.Namespace, bool) {
	for _, namespace := range connection.Namespaces {
		if namespace.Name == name {
			return namespace, true
		}
	}
	return config.Namespace{}, false
}

func stale(lastAttemptAt, now time.Time) bool {
	return now.Sub(lastAttemptAt) > StaleAfter
}

func reconcileConnection(connection fleet.Connection, enrolled config.Connection, now time.Time) fleet.Connection {
	connection.Name = enrolled.Name
	connection.Region = enrolled.Region
	connection.Namespaces = namespaceNames(enrolled)
	connection.Operators = reconcileOperators(connection, enrolled, now)
	capabilities := []fleet.Capability{}
	for _, capability := range connection.Capabilities {
		if _, ok := enrolledNamespace(enrolled, capability.Namespace); ok {
			capabilities = append(capabilities, capability)
		}
	}
	connection.Capabilities = capabilities
	if enrolled.Disabled ||
		(connection.Collection == fleet.ConnectionCollectionCurrent && stale(connection.LastAttemptAt, now)) {
		connection.Collection = fleet.ConnectionCollectionStale
	}
	return connection
}

// An observation applies to a declared controller only when every identity field
// matches. A renamed controller starts pending instead of inheriting old facts.
func reconcileOperators(connection fleet.Connection, enrolled config.Connection, now time.Time) []fleet.Operator {
	result := []fleet.Operator{}
	for _, declared := range enrolled.Operators {
		operator := fleet.Operator{
			ID:            declared.ID,
			Namespace:     declared.Namespace,
			Name:          declared.Deployment,
			Container:     declared.Container,
			Collection:    fleet.OperatorCollectionPending,
			Health:        fleet.OperatorHealthUnknown,
			LastAttemptAt: connection.LastAttemptAt,
		}
		for _, observed := range connection.Operators {
			if observed.ID == declared.ID && observed.Namespace == declared.Namespace && observed.Name == declared.Deployment &&
				observed.Container == declared.Container {
				operator = observed
				break
			}
		}
		operator.Namespaces = append([]string{}, declared.Namespaces...)
		if enrolled.Disabled ||
			(operator.Collection == fleet.OperatorCollectionCurrent && stale(operator.LastAttemptAt, now)) {
			operator.Collection = fleet.OperatorCollectionStale
		}
		result = append(result, operator)
	}
	return result
}

func buildDeployments(
	stored []fleet.Deployment,
	enrolled []config.Connection,
	connections []fleet.Connection,
	now time.Time,
) []fleet.Deployment {
	byID := enrolledByID(enrolled)
	operators := map[string][]fleet.Operator{}
	for _, connection := range connections {
		operators[connection.ID] = connection.Operators
	}
	result := []fleet.Deployment{}
	for _, deployment := range stored {
		connection, ok := byID[deployment.ConnectionID]
		if !ok {
			continue
		}
		namespace, ok := enrolledNamespace(connection, deployment.Namespace)
		if !ok {
			continue
		}
		deployment.Team, deployment.Environment, deployment.Location = namespace.Team, namespace.Environment, namespace.Location
		lastAttemptAt := deployment.Evidence.LastAttemptAt
		switch {
		case connection.Disabled:
			evidence.Stale(&deployment, lastAttemptAt, now)
		case deployment.Evidence.Collection == fleet.EvidenceCollectionCurrent && stale(lastAttemptAt, now):
			evidence.Stale(&deployment, lastAttemptAt.Add(StaleAfter), now)
		}
		fleet.AssignOperator(&deployment, operators[deployment.ConnectionID])
		result = append(result, deployment)
	}
	return result
}

// A declared relationship is shown only when both endpoints are visible deployments.
func declaredRelationships(routes []config.Relationship, deployments []fleet.Deployment) []fleet.Relationship {
	known := make(map[string]bool, len(deployments))
	for _, deployment := range deployments {
		known[deployment.ID] = true
	}
	result := []fleet.Relationship{}
	for _, route := range routes {
		source := fleet.DeploymentID(route.Source.ConnectionID, route.Source.Namespace, route.Source.Name, route.Source.UID)
		target := fleet.DeploymentID(route.Target.ConnectionID, route.Target.Namespace, route.Target.Name, route.Target.UID)
		if !known[source] || !known[target] {
			continue
		}
		relationship := fleet.Relationship{
			ID:         route.ID,
			SourceID:   source,
			TargetID:   target,
			Kind:       route.Kind,
			Provenance: fleet.ProvenanceDeclared,
		}
		if route.Repository != "" {
			repository := route.Repository
			relationship.Repository = &repository
		}
		result = append(result, relationship)
	}
	return result
}
