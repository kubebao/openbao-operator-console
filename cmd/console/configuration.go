package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kubebao/openbao-operator-console/internal/auth"
	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

func readSetupToken(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("CONSOLE_SETUP_TOKEN_FILE must reference a readable private setup token")
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o007 != 0 {
		return nil, errors.New("setup token file must be a regular file without permissions for other users")
	}
	data, err := io.ReadAll(io.LimitReader(file, 257))
	if err != nil || len(data) > 256 {
		return nil, errors.New("setup token file must contain at most 256 bytes")
	}
	return data, nil
}

func bootstrapAdministrators() []string {
	result := []string{}
	for _, value := range strings.Split(os.Getenv("CONSOLE_BOOTSTRAP_ADMIN_SUBJECTS"), ",") {
		if subject := strings.TrimSpace(value); subject != "" {
			result = append(result, subject)
		}
	}
	return result
}

func bootstrapIdentity() (*config.Identity, error) {
	identity := &config.Identity{
		Issuer:       os.Getenv("CONSOLE_OIDC_ISSUER"),
		ClientID:     os.Getenv("CONSOLE_OIDC_CLIENT_ID"),
		ClientSecret: os.Getenv("CONSOLE_OIDC_CLIENT_SECRET"),
		GroupsClaim:  "groups",
	}
	if identity.Issuer == "" || identity.ClientID == "" || identity.ClientSecret == "" {
		return nil, errors.New("bootstrap identity requires OIDC issuer, client ID, and client secret")
	}
	if file := os.Getenv("CONSOLE_OIDC_CA_FILE"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, errors.New("read bootstrap identity CA")
		}
		identity.CA = string(data)
	}
	return identity, nil
}

// bootstrapSeed permits browser identity setup when no identity seed is supplied.
// A partially supplied identity remains an installation error.
func bootstrapSeed(path string) (config.Config, error) {
	configuration := config.Config{MaxBackupAgeHours: 24, StartupGraceMinutes: 10}
	if path != "" {
		var err error
		configuration, err = config.Load(path)
		if err != nil {
			return configuration, err
		}
	}
	for _, subject := range bootstrapAdministrators() {
		configuration.GrantAdministrator(subject)
	}
	if os.Getenv("CONSOLE_OIDC_ISSUER") != "" || os.Getenv("CONSOLE_OIDC_CLIENT_ID") != "" ||
		os.Getenv("CONSOLE_OIDC_CLIENT_SECRET") != "" {
		identity, err := bootstrapIdentity()
		if err != nil {
			return configuration, err
		}
		// Seed users and groups are written for the bootstrap provider.
		configuration.SeedIdentity(*identity)
	}
	return configuration, nil
}

// recoveredConfiguration installs the recovery identity and enrolls its administrators.
// Like a browser identity change, a new issuer requires a decision about existing user
// and group access; the recovery administrators are always enrolled under the new issuer.
func recoveredConfiguration(
	current config.Config,
	identity config.Identity,
	subjects []string,
	rules config.AccessRules,
) (config.Config, string, error) {
	issuerChanged := current.AccessIssuer != identity.Issuer
	if err := current.ReplaceIdentity(identity, rules); err != nil {
		switch {
		case errors.Is(err, config.ErrAccessRulesRequired):
			return current, "", errors.New(
				"the recovery issuer differs from the stored issuer: set --recover-access-rules=carryOver or --recover-access-rules=remove",
			)
		case errors.Is(err, config.ErrAccessRulesUnexpected):
			return current, "", errors.New("the recovery issuer equals the stored issuer: omit --recover-access-rules")
		default:
			return current, "", err
		}
	}
	for _, subject := range subjects {
		current.GrantAdministrator(subject)
	}
	summary := "Identity provider and administrators recovered"
	switch {
	case issuerChanged && rules == config.AccessRulesCarryOver:
		summary += " with a new issuer; user and group access carried over"
	case issuerChanged:
		summary += " with a new issuer; user and group access removed"
	}
	return current, summary, nil
}

// Recovery is a local administrative command. It requires the existing database
// encryption key and verifies discovery before changing the active provider.
func recoverConfiguration(ctx context.Context, service *settings.Service, rules config.AccessRules) error {
	subjects := bootstrapAdministrators()
	if len(subjects) == 0 {
		return errors.New("identity recovery requires CONSOLE_BOOTSTRAP_ADMIN_SUBJECTS")
	}
	identity, err := bootstrapIdentity()
	if err != nil {
		return err
	}
	current, revision := service.Current()
	configuration, summary, err := recoveredConfiguration(current, *identity, subjects, rules)
	if err != nil {
		return err
	}
	if _, err := auth.Discover(ctx, *identity, os.Getenv("CONSOLE_OIDC_REDIRECT_URL")); err != nil {
		return fmt.Errorf("recovery identity discovery failed: %w", err)
	}
	_, err = service.Save(ctx, revision, configuration, "local-recovery", summary)
	return err
}
