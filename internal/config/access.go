package config

import (
	"errors"
	"fmt"
)

// AccessMode selects how the console reaches a cluster's Kubernetes API.
type AccessMode string

const (
	// AccessInCluster uses the console's own ServiceAccount.
	AccessInCluster AccessMode = "inCluster"
	// AccessKubeconfig loads a kubeconfig file from disk. Only the command-line check and
	// the first-start enrollment file use it; startup imports it as a remote credential.
	AccessKubeconfig AccessMode = "kubeconfig"
	// AccessRemote uses an embedded reader credential.
	AccessRemote AccessMode = "remote"
)

// Access describes how the console authenticates to one cluster.
type Access struct {
	Mode AccessMode `json:"mode"`
	// Kubeconfig and Context apply to AccessKubeconfig. An empty path with a context uses
	// the default kubeconfig loading rules.
	Kubeconfig string `json:"kubeconfig,omitempty"`
	Context    string `json:"context,omitempty"`
	// Credential applies to AccessRemote. It is stored as a separate encrypted record and
	// never serialized with the configuration.
	Credential *Credential `json:"-"`
}

func (a Access) Validate() error {
	switch a.Mode {
	case AccessInCluster:
		if a.Kubeconfig != "" || a.Context != "" || a.Credential != nil {
			return errors.New("in-cluster access takes no kubeconfig or credential")
		}
	case AccessKubeconfig:
		if a.Kubeconfig == "" && a.Context == "" {
			return errors.New("kubeconfig access requires a kubeconfig path or context")
		}
		if a.Credential != nil {
			return errors.New("kubeconfig access takes no embedded credential")
		}
	case AccessRemote:
		// Reader RBAC is generated before a credential exists. Settings refuse to save a
		// remote connection without one.
		if a.Kubeconfig != "" || a.Context != "" {
			return errors.New("remote access takes no kubeconfig")
		}
		if a.Credential != nil {
			return a.Credential.Validate()
		}
	default:
		return fmt.Errorf("unknown access mode %q", a.Mode)
	}
	return nil
}
