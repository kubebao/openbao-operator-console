package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

// problemError is a handler failure with a response safe to show administrators.
type problemError struct {
	status  int
	code    string
	message string
}

func (e *problemError) Error() string { return e.message }

// badRequest reports a request the administrator can correct.
func badRequest(message string) error {
	return &problemError{status: http.StatusBadRequest, code: "invalid", message: message}
}

// invalidRequest reports a validation error whose message is safe to show.
func invalidRequest(err error) error { return badRequest(err.Error()) }

// retryLater reports a failure the administrator can only retry.
func retryLater(message string) error {
	return &problemError{status: http.StatusServiceUnavailable, code: "unavailable", message: message}
}

// unavailable reports err as retryable. Settings conflicts and validation errors keep
// their own responses.
func unavailable(err error, message string) error {
	if errors.Is(err, settings.ErrConflict) || errors.Is(err, settings.ErrInvalid) {
		return err
	}
	return retryLater(message)
}

func writeProblem(w http.ResponseWriter, err error) {
	var response *problemError
	switch {
	case errors.As(err, &response):
		problem(w, response.status, response.code, response.message)
	case errors.Is(err, settings.ErrConflict):
		problem(w, http.StatusConflict, "conflict", "Settings changed. Reload before saving.")
	case errors.Is(err, settings.ErrInvalid):
		problem(w, http.StatusBadRequest, "invalid", err.Error())
	default:
		problem(w, http.StatusServiceUnavailable, "unavailable", "The request could not be completed. Retry later.")
	}
}

// adminHandler serves an authorized administrator. It writes its own success response
// and returns failures for admin to map.
type adminHandler func(w http.ResponseWriter, r *http.Request, actor config.User) error

// admin authorizes a console administrator and maps handler failures to problem responses.
func (s *Server) admin(handler adminHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := s.administrator(w, r)
		if !ok {
			return
		}
		if err := handler(w, r, actor); err != nil {
			writeProblem(w, err)
		}
	}
}

// administrator requires a signed-in administrator and a same-origin request for every
// method other than GET.
func (s *Server) administrator(w http.ResponseWriter, r *http.Request) (config.User, bool) {
	user, allowed := s.Auth.Viewer(r)
	if !allowed {
		problem(w, http.StatusUnauthorized, "unauthenticated", "Sign in to manage settings.")
		return user, false
	}
	if !user.Admin {
		problem(w, http.StatusForbidden, "forbidden", "Console administrator access is required.")
		return user, false
	}
	if s.Settings == nil {
		problem(w, http.StatusServiceUnavailable, "unavailable", "Settings are unavailable.")
		return user, false
	}
	if r.Method != http.MethodGet && r.Header.Get("Origin") != s.Origin {
		problem(w, http.StatusForbidden, "forbidden", "Invalid request origin.")
		return user, false
	}
	return user, true
}

// decodeBody accepts exactly one JSON object of at most 1 MiB with only known fields.
func decodeBody[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	var result T
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return result, badRequest("use application/json")
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, badRequest("request body is invalid or exceeds 1 MiB")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return result, badRequest("request must contain one JSON object")
	}
	return result, nil
}
