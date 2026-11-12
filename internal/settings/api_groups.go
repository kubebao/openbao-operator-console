package settings

import (
	"encoding/json"
	"maps"
	"regexp"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

const maxConnectionAPIGroups = 10000

var connectionBindingID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,100}$`)

// storedConfiguration preserves the original flat JSON format. Bindings belong
// only to the encrypted payload and cannot be supplied through a settings edit.
type storedConfiguration struct {
	config.Config
	APIGroups map[string]config.APIGroup `json:"connectionAPIGroups"`
}

// bindAPIGroups retains removed enrollment IDs so a later enrollment cannot join
// resources or history across operator API groups.
func bindAPIGroups(previous map[string]config.APIGroup, next config.Config) (map[string]config.APIGroup, error) {
	groups := maps.Clone(previous)
	if groups == nil {
		groups = map[string]config.APIGroup{}
	}
	for _, connection := range next.Connections {
		group := connection.EffectiveAPIGroup()
		if group != config.APIGroupOpenBao && group != config.APIGroupKubeBao {
			return nil, invalid("connection API group must be openbao.org or kubebao.org")
		}
		if previousGroup, found := groups[connection.ID]; found && previousGroup != group {
			return nil, invalid(
				"API group cannot change for connection %q; enroll the other API with a new connection identifier",
				connection.ID,
			)
		}
		groups[connection.ID] = group
	}
	if len(groups) > maxConnectionAPIGroups {
		return nil, invalid("configuration exceeds the supported lifetime connection identifier limit")
	}
	return groups, nil
}

func readAPIGroups(raw json.RawMessage, configuration config.Config) (map[string]config.APIGroup, error) {
	if raw == nil {
		// Older settings had only the legacy API. Infer current enrollments without
		// rewriting the configuration, its revision, or its audit on load.
		return bindAPIGroups(nil, configuration)
	}
	var groups map[string]config.APIGroup
	if err := json.Unmarshal(raw, &groups); err != nil || groups == nil || len(groups) > maxConnectionAPIGroups {
		return nil, invalid("stored API-group bindings are invalid")
	}
	for id, group := range groups {
		if !connectionBindingID.MatchString(id) || group != config.APIGroupOpenBao && group != config.APIGroupKubeBao {
			return nil, invalid("stored API-group bindings are invalid")
		}
	}
	for _, connection := range configuration.Connections {
		if groups[connection.ID] != connection.EffectiveAPIGroup() {
			return nil, invalid("stored API-group binding does not match enrollment")
		}
	}
	return groups, nil
}

// Retained observations from before API selection belong to the legacy API,
// including IDs removed from enrollment before this upgrade.
func legacyAPIGroups(ids []string, configuration config.Config) (map[string]config.APIGroup, error) {
	if len(ids) > maxConnectionAPIGroups {
		return nil, invalid("retained connection identities exceed the supported API-group binding limit")
	}
	groups := make(map[string]config.APIGroup, len(ids))
	for _, id := range ids {
		if !connectionBindingID.MatchString(id) {
			return nil, invalid("retained connection identity is invalid")
		}
		groups[id] = config.APIGroupOpenBao
	}
	return bindAPIGroups(groups, configuration)
}
