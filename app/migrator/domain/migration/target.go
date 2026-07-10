// Package migration is app/migrator's domain layer: the concepts this
// migrator operates on (which stack to migrate, what goose operation
// to run, and which database that stack owns), plus the ports
// (Database/Runner, see port.go) infra/postgres and infra/goose
// implement. Like app/api's domain/task and app/auth's domain
// packages, this package imports nothing outside the standard
// library -- in particular, it never imports database/sql, pgx, or
// goose, so it stays independent of any storage technology or
// migration tool (dependency inversion; see .claude/rules/db.md and
// this module's cmd/migrator, service, and infra packages).
package migration

import "fmt"

// validTargets is the closed set of Target values this domain
// accepts (SPEC-005 R5: "-target api|auth").
var validTargets = map[string]bool{"api": true, "auth": true}

// Target identifies which stack (api or auth) a migrator run applies
// migrations to. It is a value object: the zero value is never valid
// on its own, only ever produced via ParseTarget.
type Target struct {
	value string
}

// ParseTarget validates s against the closed set of recognized
// targets ("api", "auth"), rejecting anything else so an
// unrecognized value fails fast rather than silently deriving an
// unexpected default migrations directory or database name.
func ParseTarget(s string) (Target, error) {
	if !validTargets[s] {
		return Target{}, fmt.Errorf("migrator: target must be one of api, auth (got %q)", s)
	}
	return Target{value: s}, nil
}

// String returns the underlying target string ("api" or "auth").
func (t Target) String() string {
	return t.value
}

// DefaultMigrationsDir returns this target's default migrations
// directory, "/migrations/<target>", matching app/migrator/
// Dockerfile's COPY layout (COPY app/api/db/migrations /migrations/api
// and the app/auth equivalent).
func (t Target) DefaultMigrationsDir() string {
	return "/migrations/" + t.value
}
