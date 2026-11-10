// Mirrors the server rules in internal/config/local.go and internal/localauth/policy.go.
// The server remains the authority; these values only guide input.
export const localUsernamePattern = '[a-z0-9][a-z0-9._\\-]{0,63}'
export const minimumPasswordLength = 15
