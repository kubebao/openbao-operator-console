package store

import (
	"context"
	"errors"
	"net"
	"syscall"
)

// sqlState is implemented by PostgreSQL server errors. The SQLSTATE code names the class
// of failure without the server message, which can contain query parameters.
type sqlState interface{ SQLState() string }

// FailureCause classifies a failed database read or write for the log. It never returns
// driver message text.
func FailureCause(err error) string {
	var (
		database sqlState
		network  net.Error
	)
	switch {
	case err == nil:
		return "unspecified"
	case errors.Is(err, context.DeadlineExceeded):
		return "timed out"
	case errors.Is(err, context.Canceled):
		return "request canceled"
	case errors.As(err, &database):
		return "database error " + database.SQLState()
	case errors.Is(err, syscall.ECONNREFUSED):
		return "database connection refused"
	case errors.As(err, &network) && network.Timeout():
		return "database connection timed out"
	case errors.As(err, &network):
		return "database connection failed"
	default:
		return "unclassified"
	}
}
