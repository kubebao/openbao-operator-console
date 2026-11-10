package config

import "errors"

// Principal is a verified OIDC identity: the ID token issuer, subject, and group values.
type Principal struct {
	Issuer  string
	Subject string
	Groups  []string
}

// AccessRules decides what happens to user and group bindings when the identity
// provider's issuer changes. The empty value keeps bindings and is valid only while the
// issuer stays the same.
type AccessRules string

const (
	// AccessRulesCarryOver keeps every user and group binding and binds it to the new issuer.
	AccessRulesCarryOver AccessRules = "carryOver"
	// AccessRulesRemove removes every user and group binding.
	AccessRulesRemove AccessRules = "remove"
)

// Valid reports whether r is a known decision or no decision.
func (r AccessRules) Valid() bool {
	return r == "" || r == AccessRulesCarryOver || r == AccessRulesRemove
}

var (
	// ErrAccessRulesRequired reports an issuer change without a decision about existing bindings.
	ErrAccessRulesRequired = errors.New(
		"the issuer changes: choose whether existing user and group access rules carry over or are removed",
	)
	// ErrAccessRulesUnexpected reports a binding decision for an unchanged issuer.
	ErrAccessRulesUnexpected = errors.New("access rules carry over or removal applies only when the issuer changes")
)

// SeedIdentity installs the first identity provider. Seed users and groups are written for
// it, so they bind to its issuer.
func (c *Config) SeedIdentity(identity Identity) {
	c.Identity = &identity
	c.AccessIssuer = identity.Issuer
}

// ReplaceIdentity activates a different identity provider configuration. Bindings stay
// without a decision only when the issuer is unchanged. Otherwise rules must carry them
// over to the new issuer or remove them; a binding never moves to another issuer silently.
// A console without a provider, which uses local accounts only, takes its first provider
// without a decision: seeded bindings were written for it, as in browser setup.
func (c *Config) ReplaceIdentity(identity Identity, rules AccessRules) error {
	switch {
	case !rules.Valid():
		return errors.New("unknown access rules decision")
	case c.Identity == nil && c.AccessIssuer == "":
		if rules != "" {
			return ErrAccessRulesUnexpected
		}
	case identity.Issuer == c.AccessIssuer:
		if rules != "" {
			return ErrAccessRulesUnexpected
		}
	case rules == AccessRulesCarryOver:
	case rules == AccessRulesRemove:
		c.Users = []User{}
		c.Groups = []Group{}
	default:
		return ErrAccessRulesRequired
	}
	c.Identity = &identity
	c.AccessIssuer = identity.Issuer
	return nil
}
