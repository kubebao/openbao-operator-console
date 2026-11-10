package settings

import (
	"context"
	"slices"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

// change applies one edit to the current configuration with revision compare-and-swap.
// The edit returns the summary for the change log.
func (s *Service) change(
	ctx context.Context,
	revision int64,
	actor string,
	edit func(*config.Config) (string, error),
) (int64, error) {
	s.commit.Lock()
	defer s.commit.Unlock()
	if revision != s.revision {
		return 0, ErrConflict
	}
	next := clone(s.configuration)
	summary, err := edit(&next)
	if err != nil {
		return 0, err
	}
	return s.save(ctx, revision, next, actor, summary, nil)
}

func localAccountIndex(c *config.Config, id string) (int, error) {
	index := slices.IndexFunc(c.LocalAccounts, func(account config.LocalAccount) bool { return account.ID == id })
	if index < 0 {
		return 0, invalid("local account is not enrolled")
	}
	return index, nil
}

// AddLocalAccount enrolls a new local account. Its credential is stored separately.
func (s *Service) AddLocalAccount(ctx context.Context, revision int64, account config.LocalAccount, actor string) (int64, error) {
	return s.change(ctx, revision, actor, func(c *config.Config) (string, error) {
		if _, exists := c.FindLocalAccount(account.Username); exists {
			return "", invalid("username %q is already enrolled", account.Username)
		}
		c.LocalAccounts = append(c.LocalAccounts, account)
		return "Added local account " + account.Username, nil
	})
}

// UpdateLocalAccount replaces the name, administrator flag, and scopes of an account.
// The ID and username stay.
func (s *Service) UpdateLocalAccount(ctx context.Context, revision int64, update config.LocalAccount, actor string) (int64, error) {
	return s.change(ctx, revision, actor, func(c *config.Config) (string, error) {
		index, err := localAccountIndex(c, update.ID)
		if err != nil {
			return "", err
		}
		account := &c.LocalAccounts[index]
		account.Name, account.Admin, account.Scopes = update.Name, update.Admin, cloneScopes(update.Scopes)
		return "Updated local account " + account.Username, nil
	})
}

// RemoveLocalAccount removes an account from the configuration. The caller removes its
// credential afterwards.
func (s *Service) RemoveLocalAccount(ctx context.Context, revision int64, id, actor string) (int64, error) {
	return s.change(ctx, revision, actor, func(c *config.Config) (string, error) {
		index, err := localAccountIndex(c, id)
		if err != nil {
			return "", err
		}
		username := c.LocalAccounts[index].Username
		c.LocalAccounts = slices.Delete(c.LocalAccounts, index, index+1)
		return "Removed local account " + username, nil
	})
}

// SetLocalSignIn turns local sign-in on or off. Turning it off requires an identity
// provider and an administrator who signs in through it.
func (s *Service) SetLocalSignIn(ctx context.Context, revision int64, enabled bool, actor string) (int64, error) {
	return s.change(ctx, revision, actor, func(c *config.Config) (string, error) {
		c.LocalSignInDisabled = !enabled
		if enabled {
			return "Enabled local sign-in", nil
		}
		return "Disabled local sign-in", nil
	})
}
