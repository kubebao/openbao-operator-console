package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

// maxTokenLength bounds the bearer tokens the console decodes.
const maxTokenLength = 16384

// tokenClaims decodes the payload of a JWT without verifying its signature. Callers use
// the claims only to select a credential mode or show a declared expiry; Kubernetes
// authenticates every request.
func tokenClaims(token string, claims any) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || len(token) > maxTokenLength {
		return errors.New("token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return errors.New("token is not a JWT")
	}
	return json.Unmarshal(payload, claims)
}
