package postgres

import (
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Safety ceilings for the production Open path. They are database-side
// cancellation backstops, NOT calibrated latency SLOs or permission to retry
// an external effect after an ambiguous commit. Schema migration and backup
// remain independently controlled deployment operations.
//
// Pilot/test stores created from a supplied pool with New(pool) retain the
// caller's explicit connection policy; production Control calls Open.
const (
	maxStatementTimeout  = 20 * time.Second
	maxLockTimeout       = 5 * time.Second
	maxIdleInTransaction = 20 * time.Second
)

type databaseDeadline struct {
	name  string
	limit time.Duration
}

var databaseDeadlines = []databaseDeadline{
	{"statement_timeout", maxStatementTimeout},
	{"lock_timeout", maxLockTimeout},
	{"idle_in_transaction_session_timeout", maxIdleInTransaction},
}

// applyConnectionDeadlines keeps explicit *stricter* administrator limits.
// Invalid, disabled or looser limits fail closed, not silently widened.
// A missing setting receives the conservative exact ceiling in milliseconds.
func applyConnectionDeadlines(config *pgxpool.Config) error {
	if config == nil || config.ConnConfig == nil {
		return fmt.Errorf("PostgreSQL connection configuration is required")
	}
	params := config.ConnConfig.RuntimeParams
	if params == nil {
		params = make(map[string]string)
		config.ConnConfig.RuntimeParams = params
	}
	for _, item := range databaseDeadlines {
		raw, exists := params[item.name]
		if !exists {
			params[item.name] = strconv.FormatInt(item.limit.Milliseconds(), 10)
			continue
		}
		duration, err := pgParameterDuration(raw)
		if err != nil || duration <= 0 || duration > item.limit {
			return fmt.Errorf("PostgreSQL %s must be nonzero and no greater than %s", item.name, item.limit)
		}
	}
	return nil
}

// PostgreSQL bare timeout integers are in milliseconds. Explicit Go-style
// duration suffixes (ms, s, m, h) are accepted. Unsupported units fail closed;
// operators can use an integer number of milliseconds instead.
func pgParameterDuration(value string) (time.Duration, error) {
	if value == "" {
		return 0, fmt.Errorf("empty PostgreSQL timeout")
	}
	if milliseconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if milliseconds < 0 || milliseconds > int64((24*time.Hour).Milliseconds()) {
			return 0, fmt.Errorf("PostgreSQL timeout out of range")
		}
		return time.Duration(milliseconds) * time.Millisecond, nil
	}
	return time.ParseDuration(value)
}
