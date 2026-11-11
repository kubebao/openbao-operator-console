// Package store retains approved observations and durable issue first-seen times.
package store

import (
	"time"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

// Batch is the result of one collection round for one connection.
type Batch struct {
	CompletedAt time.Time
	// Unresolved retains prior issues only for rules whose fresh evaluation is incomplete.
	Unresolved  map[string][]fleet.Rule
	Connection  fleet.Connection
	Deployments []fleet.Deployment
}
