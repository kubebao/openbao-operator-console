// Package config defines administrator-controlled enrollment and access scopes.
package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/validation"
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

func (c Config) Validate() error {
	if c.MaxBackupAgeHours <= 0 || c.MaxBackupAgeHours > 87600 || c.StartupGraceMinutes < 0 || c.StartupGraceMinutes > 1440 {
		return errors.New("backup age must be 1–87600 hours and startup grace must be 0–1440 minutes")
	}
	if len(c.Connections) > 1000 || len(c.Users) > 10000 || len(c.Groups) > 1000 || len(c.Relationships) > 10000 {
		return errors.New("configuration exceeds the supported enrollment limits")
	}
	identifier := regexp.MustCompile(`^[a-zA-Z0-9_-]{1,100}$`)
	connections := make(map[string]map[string]bool, len(c.Connections))
	for _, connection := range c.Connections {
		if err := connection.ValidateRenewal(); err != nil {
			return err
		}
		if !identifier.MatchString(connection.ID) || connection.Name == "" || len(connection.Namespaces) == 0 ||
			len(connection.Namespaces) > 1000 {
			return errors.New("connection requires id, name, and at least one namespace")
		}
		if _, exists := connections[connection.ID]; exists {
			return fmt.Errorf("duplicate connection id %q", connection.ID)
		}
		namespaces := make(map[string]bool)
		for _, namespace := range connection.Namespaces {
			if len(validation.IsDNS1123Label(namespace.Name)) != 0 || namespaces[namespace.Name] {
				return fmt.Errorf("invalid or duplicate namespace in connection %q", connection.ID)
			}
			if !namespace.Environment.Valid() {
				return fmt.Errorf("invalid environment in connection %q", connection.ID)
			}
			namespaces[namespace.Name] = true
		}
		connections[connection.ID] = namespaces
		if err := connection.Access.Validate(); err != nil {
			return fmt.Errorf("connection %q: %w", connection.ID, err)
		}
		operators := make(map[string]bool)
		assigned := make(map[string]bool)
		for _, operator := range connection.Operators {
			if operator.ID == "" || operators[operator.ID] || operator.Namespace == "" || operator.Deployment == "" ||
				operator.Container == "" ||
				len(operator.Namespaces) == 0 {
				return errors.New("operator requires a unique id, namespace, deployment, container, and managed namespaces")
			}
			operators[operator.ID] = true
			if len(validation.IsDNS1123Label(operator.Namespace)) != 0 || len(validation.IsDNS1123Subdomain(operator.Deployment)) != 0 ||
				len(validation.IsDNS1123Label(operator.Container)) != 0 {
				return errors.New("operator enrollment requires valid Kubernetes names")
			}
			for _, namespace := range operator.Namespaces {
				if !namespaces[namespace] || assigned[namespace] {
					return fmt.Errorf("operator namespace must be enrolled and assigned once in connection %q", connection.ID)
				}
				assigned[namespace] = true
			}
		}
	}
	users := make(map[string]bool)
	for _, user := range c.Users {
		if user.Subject == "" || len(user.Subject) > 1024 || users[user.Subject] {
			return errors.New("invalid or duplicate subject")
		}
		users[user.Subject] = true
		for _, scope := range user.Scopes {
			for _, namespace := range scope.Namespaces {
				if !connections[scope.ConnectionID][namespace] {
					return errors.New("user scope references an unenrolled namespace")
				}
			}
		}
	}
	groups := make(map[string]bool)
	for _, group := range c.Groups {
		if group.Name == "" || len(group.Name) > 1024 || groups[group.Name] {
			return errors.New("group requires a unique name")
		}
		groups[group.Name] = true
		for _, scope := range group.Scopes {
			for _, namespace := range scope.Namespaces {
				if !connections[scope.ConnectionID][namespace] {
					return errors.New("group scope references an unenrolled namespace")
				}
			}
		}
	}
	routes := make(map[string]bool)
	for _, route := range c.Relationships {
		if !identifier.MatchString(route.ID) || routes[route.ID] || route.Source == route.Target {
			return errors.New("invalid or duplicate relationship id")
		}
		routes[route.ID] = true
		if !route.Kind.Valid() {
			return errors.New("invalid relationship kind")
		}
		for _, resource := range []ResourceRef{route.Source, route.Target} {
			if resource.Name == "" || resource.UID == "" || !connections[resource.ConnectionID][resource.Namespace] {
				return errors.New("relationship requires enrolled resource identities and UIDs")
			}
		}
		if strings.Contains(route.Repository, "@") || strings.Contains(route.Repository, "?") {
			return errors.New("repository display value must not contain userinfo or query parameters")
		}
	}
	return nil
}
