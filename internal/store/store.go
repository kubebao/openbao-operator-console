// Package store retains approved observations and durable issue first-seen times.
package store

import "github.com/kubebao/openbao-operator-console/internal/fleet"

// Batch is the result of one collection round for one connection.
type Batch struct {
	Connection  fleet.Connection
	Deployments []fleet.Deployment
}
