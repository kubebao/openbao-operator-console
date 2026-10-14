package config

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

var identifier = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,100}$`)

// enrolledNamespaces records the namespaces of each connection by connection ID.
type enrolledNamespaces map[string]map[string]bool

func (e enrolledNamespaces) contains(connectionID, namespace string) bool {
	return e[connectionID][namespace]
}

// Validate checks limits, identities, and references. Every scope and relationship must
// name an enrolled namespace.
func (c Config) Validate() error {
	if c.MaxBackupAgeHours <= 0 || c.MaxBackupAgeHours > 87600 || c.StartupGraceMinutes < 0 || c.StartupGraceMinutes > 1440 {
		return errors.New("backup age must be 1–87600 hours and startup grace must be 0–1440 minutes")
	}
	if len(c.Connections) > 1000 || len(c.Users) > 10000 || len(c.Groups) > 1000 || len(c.Relationships) > 10000 {
		return errors.New("configuration exceeds the supported enrollment limits")
	}
	issuer := ""
	if c.Identity != nil {
		issuer = c.Identity.Issuer
	}
	if c.AccessIssuer != issuer {
		return errors.New("user and group access must belong to the active identity provider's issuer")
	}
	enrolled := enrolledNamespaces{}
	for _, connection := range c.Connections {
		namespaces, err := connection.validate()
		if err != nil {
			return err
		}
		if _, exists := enrolled[connection.ID]; exists {
			return fmt.Errorf("duplicate connection id %q", connection.ID)
		}
		enrolled[connection.ID] = namespaces
	}
	if err := validateUsers(c.Users, enrolled); err != nil {
		return err
	}
	if err := validateGroups(c.Groups, enrolled); err != nil {
		return err
	}
	return validateRelationships(c.Relationships, enrolled)
}

// validate checks one connection and returns its enrolled namespaces.
func (c Connection) validate() (map[string]bool, error) {
	if err := c.ValidateRenewal(); err != nil {
		return nil, err
	}
	if !identifier.MatchString(c.ID) || c.Name == "" || len(c.Namespaces) == 0 || len(c.Namespaces) > 1000 {
		return nil, errors.New("connection requires id, name, and at least one namespace")
	}
	namespaces := make(map[string]bool, len(c.Namespaces))
	for _, namespace := range c.Namespaces {
		if len(validation.IsDNS1123Label(namespace.Name)) != 0 || namespaces[namespace.Name] {
			return nil, fmt.Errorf("invalid or duplicate namespace in connection %q", c.ID)
		}
		if !namespace.Environment.Valid() {
			return nil, fmt.Errorf("invalid environment in connection %q", c.ID)
		}
		namespaces[namespace.Name] = true
	}
	if err := c.Access.Validate(); err != nil {
		return nil, fmt.Errorf("connection %q: %w", c.ID, err)
	}
	return namespaces, c.validateOperators(namespaces)
}

// Each enrolled namespace has at most one responsible controller.
func (c Connection) validateOperators(namespaces map[string]bool) error {
	operators := make(map[string]bool, len(c.Operators))
	assigned := make(map[string]bool)
	for _, operator := range c.Operators {
		if operator.ID == "" || operators[operator.ID] || operator.Namespace == "" || operator.Deployment == "" ||
			operator.Container == "" || len(operator.Namespaces) == 0 {
			return errors.New("operator requires a unique id, namespace, deployment, container, and managed namespaces")
		}
		operators[operator.ID] = true
		if len(validation.IsDNS1123Label(operator.Namespace)) != 0 || len(validation.IsDNS1123Subdomain(operator.Deployment)) != 0 ||
			len(validation.IsDNS1123Label(operator.Container)) != 0 {
			return errors.New("operator enrollment requires valid Kubernetes names")
		}
		for _, namespace := range operator.Namespaces {
			if !namespaces[namespace] || assigned[namespace] {
				return fmt.Errorf("operator namespace must be enrolled and assigned once in connection %q", c.ID)
			}
			assigned[namespace] = true
		}
	}
	return nil
}

func validateScopes(scopes []Scope, enrolled enrolledNamespaces) bool {
	for _, scope := range scopes {
		for _, namespace := range scope.Namespaces {
			if !enrolled.contains(scope.ConnectionID, namespace) {
				return false
			}
		}
	}
	return true
}

func validateUsers(users []User, enrolled enrolledNamespaces) error {
	subjects := make(map[string]bool, len(users))
	for _, user := range users {
		if user.Subject == "" || len(user.Subject) > 1024 || subjects[user.Subject] {
			return errors.New("invalid or duplicate subject")
		}
		subjects[user.Subject] = true
		if !validateScopes(user.Scopes, enrolled) {
			return errors.New("user scope references an unenrolled namespace")
		}
	}
	return nil
}

func validateGroups(groups []Group, enrolled enrolledNamespaces) error {
	names := make(map[string]bool, len(groups))
	for _, group := range groups {
		if group.Name == "" || len(group.Name) > 1024 || names[group.Name] {
			return errors.New("group requires a unique name")
		}
		names[group.Name] = true
		if !validateScopes(group.Scopes, enrolled) {
			return errors.New("group scope references an unenrolled namespace")
		}
	}
	return nil
}

func validateRelationships(routes []Relationship, enrolled enrolledNamespaces) error {
	ids := make(map[string]bool, len(routes))
	for _, route := range routes {
		if !identifier.MatchString(route.ID) || ids[route.ID] || route.Source == route.Target {
			return errors.New("invalid or duplicate relationship id")
		}
		ids[route.ID] = true
		if !route.Kind.Valid() {
			return errors.New("invalid relationship kind")
		}
		for _, resource := range []ResourceRef{route.Source, route.Target} {
			if resource.Name == "" || resource.UID == "" || !enrolled.contains(resource.ConnectionID, resource.Namespace) {
				return errors.New("relationship requires enrolled resource identities and UIDs")
			}
		}
		if strings.Contains(route.Repository, "@") || strings.Contains(route.Repository, "?") {
			return errors.New("repository display value must not contain userinfo or query parameters")
		}
	}
	return nil
}
