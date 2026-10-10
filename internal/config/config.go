// Package config defines administrator-controlled enrollment and access scopes.
package config

import (
	"fmt"
	"os"
	"slices"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

type Namespace struct {
	Name        string            `json:"name"`
	Team        string            `json:"team"`
	Environment fleet.Environment `json:"environment"`
	Location    string            `json:"location"`
}

type Connection struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Region     string        `json:"region"`
	Access     Access        `json:"access"`
	Namespaces []Namespace   `json:"namespaces"`
	Operators  []Operator    `json:"operators"`
	Disabled   bool          `json:"disabled,omitempty"`
	Renewal    *TokenRenewal `json:"renewal,omitempty"`
}

// Operator binds one named controller Deployment to enrolled namespaces.
// Enrollment declares responsibility; workload readiness does not prove reconciliation.
type Operator struct {
	ID         string   `json:"id"`
	Namespace  string   `json:"namespace"`
	Deployment string   `json:"deployment"`
	Container  string   `json:"container"`
	Namespaces []string `json:"namespaces"`
}

type Scope struct {
	ConnectionID string   `json:"connectionId"`
	Namespaces   []string `json:"namespaces"`
}

type User struct {
	Subject string  `json:"subject"`
	Name    string  `json:"name"`
	Scopes  []Scope `json:"scopes"`
	Admin   bool    `json:"admin,omitempty"`
}

// GrantAdministrator makes a subject a console administrator, enrolling it when absent.
// It never modifies the caller's user slice in place.
func (c *Config) GrantAdministrator(subject string) {
	c.Users = slices.Clone(c.Users)
	for i := range c.Users {
		if c.Users[i].Subject == subject {
			c.Users[i].Admin = true
			return
		}
	}
	c.Users = append(c.Users, User{Subject: subject, Name: subject, Admin: true, Scopes: []Scope{}})
}

// CanView reports whether the user's scopes include a namespace of a connection.
func (u User) CanView(connectionID, namespace string) bool {
	for _, scope := range u.Scopes {
		if scope.ConnectionID == connectionID && slices.Contains(scope.Namespaces, namespace) {
			return true
		}
	}
	return false
}

type Group struct {
	Name   string  `json:"name"`
	Admin  bool    `json:"admin"`
	Scopes []Scope `json:"scopes"`
}

type Identity struct {
	Issuer       string `json:"issuer"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	CA           string `json:"ca"`
	GroupsClaim  string `json:"groupsClaim"`
}

// ResourceRef requires the original UID. A replacement never inherits a route.
type ResourceRef struct {
	ConnectionID string `json:"connectionId"`
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	UID          string `json:"uid"`
}

type Relationship struct {
	ID         string                 `json:"id"`
	Source     ResourceRef            `json:"source"`
	Target     ResourceRef            `json:"target"`
	Kind       fleet.RelationshipKind `json:"kind"`
	Repository string                 `json:"repository"`
}

type Config struct {
	Connections         []Connection   `json:"connections"`
	Users               []User         `json:"users"`
	Relationships       []Relationship `json:"relationships"`
	MaxBackupAgeHours   int            `json:"maxBackupAgeHours"`
	StartupGraceMinutes int            `json:"startupGraceMinutes"`
	Groups              []Group        `json:"groups,omitempty"`
	Identity            *Identity      `json:"identity,omitempty"`
}

// Policy returns the attention thresholds that evidence rules apply.
func (c Config) Policy() fleet.Policy {
	return fleet.Policy{
		MaxBackupAge: time.Duration(c.MaxBackupAgeHours) * time.Hour,
		StartupGrace: time.Duration(c.StartupGraceMinutes) * time.Minute,
	}
}

// enrollmentFile is the first-start YAML format. A connection names a kubeconfig file,
// a context, or neither for the console's in-cluster ServiceAccount.
type enrollmentFile struct {
	Connections         []enrollmentConnection `json:"connections"`
	Users               []User                 `json:"users"`
	Groups              []Group                `json:"groups,omitempty"`
	Relationships       []Relationship         `json:"relationships"`
	MaxBackupAgeHours   int                    `json:"maxBackupAgeHours"`
	StartupGraceMinutes int                    `json:"startupGraceMinutes"`
	Identity            *Identity              `json:"identity,omitempty"`
}

type enrollmentConnection struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Region     string        `json:"region"`
	Kubeconfig string        `json:"kubeconfig"`
	Context    string        `json:"context"`
	Namespaces []Namespace   `json:"namespaces"`
	Operators  []Operator    `json:"operators"`
	Disabled   bool          `json:"disabled,omitempty"`
	Renewal    *TokenRenewal `json:"renewal,omitempty"`
}

func (f enrollmentFile) config() Config {
	result := Config{
		Connections:         []Connection{},
		Users:               f.Users,
		Groups:              f.Groups,
		Relationships:       f.Relationships,
		MaxBackupAgeHours:   f.MaxBackupAgeHours,
		StartupGraceMinutes: f.StartupGraceMinutes,
		Identity:            f.Identity,
	}
	for _, c := range f.Connections {
		access := Access{Mode: AccessInCluster}
		if c.Kubeconfig != "" || c.Context != "" {
			access = Access{Mode: AccessKubeconfig, Kubeconfig: c.Kubeconfig, Context: c.Context}
		}
		result.Connections = append(result.Connections, Connection{
			ID:         c.ID,
			Name:       c.Name,
			Region:     c.Region,
			Access:     access,
			Namespaces: c.Namespaces,
			Operators:  c.Operators,
			Disabled:   c.Disabled,
			Renewal:    c.Renewal,
		})
	}
	return result
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read enrollment: %w", err)
	}
	var file enrollmentFile
	if err := yaml.UnmarshalStrict(data, &file); err != nil {
		return Config{}, fmt.Errorf("decode enrollment: %w", err)
	}
	result := file.config()
	if err := result.Validate(); err != nil {
		return Config{}, err
	}
	return result, nil
}
