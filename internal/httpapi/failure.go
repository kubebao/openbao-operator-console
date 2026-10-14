package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kubebao/openbao-operator-console/internal/settings"
	"github.com/kubebao/openbao-operator-console/internal/store"
)

// failureCause classifies a server-side failure for the log. It never returns driver or
// provider message text.
func failureCause(err error) string {
	if errors.Is(err, settings.ErrMissing) {
		return "configuration not initialized"
	}
	return store.FailureCause(err)
}

// route names the matched route pattern without its method, or the path class when no
// pattern matched. It never includes path values or query strings.
func route(r *http.Request) string {
	if r.Pattern == "" {
		return "unmatched"
	}
	if _, path, found := strings.Cut(r.Pattern, " "); found {
		return path
	}
	return r.Pattern
}
