package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/auth"
	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/localauth"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

var localUsername = regexp.MustCompile(config.LocalUsernamePattern)

// recoverLocalAdministrator restores administrator access without an identity provider.
// It creates or promotes the local account, enables local sign-in, revokes the account's
// credential, and prints a one-time enrollment link. The serving console must be stopped.
func recoverLocalAdministrator(
	ctx context.Context,
	service *settings.Service,
	local *localauth.Authenticator,
	username string,
	out io.Writer,
) error {
	if !localUsername.MatchString(username) {
		return errors.New("recover-local-administrator requires a lowercase username of letters, digits, dots, hyphens, or underscores")
	}
	callback := environment("CONSOLE_OIDC_REDIRECT_URL")
	if err := auth.ValidCallback(callback); err != nil {
		return fmt.Errorf("CONSOLE_OIDC_REDIRECT_URL names the public address for the link: %w", err)
	}
	public, _ := url.Parse(callback)
	current, revision := service.Current()
	account, found := current.FindLocalAccount(username)
	if !found {
		id, err := config.NewLocalAccountID()
		if err != nil {
			return err
		}
		account = config.LocalAccount{ID: id, Username: username, Scopes: []config.Scope{}}
		current.LocalAccounts = append(current.LocalAccounts, account)
	}
	for i := range current.LocalAccounts {
		if current.LocalAccounts[i].ID == account.ID {
			current.LocalAccounts[i].Admin = true
		}
	}
	current.LocalSignInDisabled = false
	if _, err := service.Save(ctx, revision, current, "local-recovery", "Local administrator "+username+" recovered"); err != nil {
		return fmt.Errorf("save recovered administrator: %w", err)
	}
	link, err := local.IssueLink(ctx, account.ID)
	if err != nil {
		return fmt.Errorf("issue enrollment link: %w", err)
	}
	_, err = fmt.Fprintf(
		out,
		"Local administrator %s recovered. Open this one-time link before %s to set a password and "+
			"authenticator:\n%s://%s/enroll#token=%s\n",
		username,
		link.ExpiresAt.Format(time.RFC3339),
		public.Scheme,
		public.Host,
		link.Token,
	)
	return err
}
