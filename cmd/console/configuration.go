package main

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kubebao/openbao-operator-console/internal/auth"
	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

// environment reads a deployment setting. Values copied from Secret files often end in a
// newline, so surrounding whitespace is removed.
func environment(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

var errConfigKey = errors.New(
	"configuration key must contain 32 bytes, stored raw or encoded as 64 hexadecimal or 44 base64 characters",
)

// readConfigKey reads the configuration encryption key from CONSOLE_CONFIG_KEY_FILE.
func readConfigKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("CONSOLE_CONFIG_KEY_FILE must reference a readable configuration key")
	}
	return decodeConfigKey(data)
}

// decodeConfigKey accepts 32 raw bytes, or 32 bytes encoded as hexadecimal or base64 text
// for secret stores that hold text only. The length distinguishes the forms. A 32-byte
// value of printable text, such as a password or 16 hexadecimal-encoded bytes, is
// rejected: random raw bytes are almost never all printable.
func decodeConfigKey(data []byte) ([]byte, error) {
	if len(data) == 32 {
		if printable(data) {
			return nil, errConfigKey
		}
		return data, nil
	}
	text := strings.TrimRight(strings.TrimSpace(string(data)), "=")
	var key []byte
	var err error
	switch len(text) {
	case 64:
		key, err = hex.DecodeString(text)
	case 43:
		key, err = base64.RawStdEncoding.DecodeString(text)
		if err != nil {
			key, err = base64.RawURLEncoding.DecodeString(text)
		}
	default:
		return nil, errConfigKey
	}
	if err != nil || len(key) != 32 {
		return nil, errConfigKey
	}
	return key, nil
}

func printable(data []byte) bool {
	for _, b := range data {
		if b < 0x20 || b > 0x7e {
			return false
		}
	}
	return true
}

func readSetupToken(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New(
			"CONSOLE_SETUP_TOKEN_FILE must reference a readable private setup token: an empty database needs one " +
				"for browser setup (the chart's SETUP_TOKEN key), or a bootstrap identity",
		)
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
	for _, value := range strings.Split(environment("CONSOLE_BOOTSTRAP_ADMIN_SUBJECTS"), ",") {
		if subject := strings.TrimSpace(value); subject != "" {
			result = append(result, subject)
		}
	}
	return result
}

func bootstrapIdentity() (*config.Identity, error) {
	identity := &config.Identity{
		Issuer:       environment("CONSOLE_OIDC_ISSUER"),
		ClientID:     environment("CONSOLE_OIDC_CLIENT_ID"),
		ClientSecret: environment("CONSOLE_OIDC_CLIENT_SECRET"),
		GroupsClaim:  "groups",
	}
	if identity.Issuer == "" || identity.ClientID == "" || identity.ClientSecret == "" {
		return nil, errors.New("bootstrap identity requires OIDC issuer, client ID, and client secret")
	}
	if file := environment("CONSOLE_OIDC_CA_FILE"); file != "" {
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
	if environment("CONSOLE_OIDC_ISSUER") != "" || environment("CONSOLE_OIDC_CLIENT_ID") != "" ||
		environment("CONSOLE_OIDC_CLIENT_SECRET") != "" {
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
	if _, err := auth.Discover(ctx, *identity, environment("CONSOLE_OIDC_REDIRECT_URL")); err != nil {
		return fmt.Errorf("recovery identity discovery failed: %w", err)
	}
	_, err = service.Save(ctx, revision, configuration, "local-recovery", summary)
	return err
}
