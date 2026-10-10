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
		configuration.Identity = identity
	}
	return configuration, nil
}

// Recovery is a local administrative command. It requires the existing database
// encryption key and verifies discovery before changing the active provider.
func recoverConfiguration(ctx context.Context, service *settings.Service) error {
	subjects := bootstrapAdministrators()
	if len(subjects) == 0 {
		return errors.New("identity recovery requires CONSOLE_BOOTSTRAP_ADMIN_SUBJECTS")
	}
	identity, err := bootstrapIdentity()
	if err != nil {
		return err
	}
	if _, err := auth.Discover(ctx, *identity, os.Getenv("CONSOLE_OIDC_REDIRECT_URL"), nil); err != nil {
		return fmt.Errorf("recovery identity discovery failed: %w", err)
	}
	configuration, revision := service.Current()
	for _, subject := range subjects {
		configuration.GrantAdministrator(subject)
	}
	configuration.Identity = identity
	_, err = service.Save(ctx, revision, configuration, "local-recovery", "Identity provider and administrators recovered")
	return err
}
