package store

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestFailureCauseNamesTheClassWithoutDriverText(t *testing.T) {
	for _, test := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("apply batch: %w", &pgconn.PgError{Code: "40001", Message: "secret detail"}), "database error 40001"},
		{fmt.Errorf("apply batch: %w", context.DeadlineExceeded), "timed out"},
		{errors.New("raw driver text"), "unclassified"},
	} {
		if got := FailureCause(test.err); got != test.want {
			t.Errorf("FailureCause(%v) = %q, want %q", test.err, got, test.want)
		}
	}
}
