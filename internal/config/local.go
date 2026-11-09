package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
)

// LocalIssuer marks principals that signed in with a local account. OIDC issuers are
// HTTPS URLs, so it never equals one, and local accounts never match OIDC bindings.
const LocalIssuer = "console:local"

// maxLocalAccounts bounds local accounts. Larger installations should use OIDC.
const maxLocalAccounts = 1000

// LocalUsernamePattern allows lowercase names that are safe in logs and URLs.
const LocalUsernamePattern = `^[a-z0-9][a-z0-9._-]{0,63}$`

var localUsername = regexp.MustCompile(LocalUsernamePattern)

// LocalAccount is a console-managed user who signs in with a password and TOTP. The
// password hash and TOTP secret are stored separately in encrypted form, keyed by ID.
type LocalAccount struct {
	// ID is random and immutable. A deleted account's credential therefore never
	// applies to a new account with the same username.
	ID       string  `json:"id"`
	Username string  `json:"username"`
	Name     string  `json:"name"`
	Scopes   []Scope `json:"scopes"`
	Admin    bool    `json:"admin,omitempty"`
}

// NewLocalAccountID returns a random account identifier.
func NewLocalAccountID() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

// LocalSubject is the actor recorded in the change log for a local account.
func LocalSubject(username string) string { return "local:" + username }

// FindLocalAccount returns the account with username, ignoring case.
func (c Config) FindLocalAccount(username string) (LocalAccount, bool) {
	username = strings.ToLower(strings.TrimSpace(username))
	for _, account := range c.LocalAccounts {
		if account.Username == username {
			return account, true
		}
	}
	return LocalAccount{}, false
}

// LocalAccountIDs lists the IDs of every local account.
func (c Config) LocalAccountIDs() []string {
	ids := make([]string, 0, len(c.LocalAccounts))
	for _, account := range c.LocalAccounts {
		ids = append(ids, account.ID)
	}
	return ids
}

// resolveLocal finds the account a local principal names by its ID.
func (c Config) resolveLocal(id string) (User, bool) {
	for _, account := range c.LocalAccounts {
		if account.ID == id {
			name := account.Name
			if name == "" {
				name = account.Username
			}
			return User{
				Subject: LocalSubject(account.Username),
				Name:    name,
				Admin:   account.Admin,
				Scopes:  append([]Scope{}, account.Scopes...),
			}, true
		}
	}
	return User{Subject: "", Name: "", Scopes: []Scope{}}, false
}

func validateLocalAccounts(accounts []LocalAccount, enrolled enrolledNamespaces) error {
	if len(accounts) > maxLocalAccounts {
		return errors.New("configuration exceeds the supported number of local accounts")
	}
	ids := make(map[string]bool, len(accounts))
	usernames := make(map[string]bool, len(accounts))
	for _, account := range accounts {
		if !identifier.MatchString(account.ID) || ids[account.ID] {
			return errors.New("local account requires a unique identifier")
		}
		if !localUsername.MatchString(account.Username) || usernames[account.Username] {
			return errors.New("local account requires a unique lowercase username")
		}
		if len(account.Name) > 256 {
			return errors.New("local account name is too long")
		}
		ids[account.ID] = true
		usernames[account.Username] = true
		if !validateScopes(account.Scopes, enrolled) {
			return errors.New("local account scope references an unenrolled namespace")
		}
	}
	return nil
}
