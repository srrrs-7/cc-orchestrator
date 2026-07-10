package service

import (
	"context"
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/migrator/domain/migration"
)

// callRecorder records the order in which the fake ports below are
// invoked during a single Service.Migrate call, so tests can assert
// not just "was EnsureAppRole called" but "was it called after Run
// succeeded" -- ISSUE-016 R-c's ordering requirement (service.go's
// Migrate doc comment: "GRANT は goose up の後に流す").
type callRecorder struct {
	calls []string
}

func (r *callRecorder) record(name string) {
	r.calls = append(r.calls, name)
}

// fakeDatabase is a test double for migration.Database: it records
// every EnsureExists call (via rec, when non-nil) and returns err,
// letting tests simulate a database-bootstrap failure.
type fakeDatabase struct {
	rec *callRecorder
	err error

	calls int
}

func (f *fakeDatabase) EnsureExists(ctx context.Context, name migration.DatabaseName) error {
	f.calls++
	if f.rec != nil {
		f.rec.record("ensure_exists")
	}
	return f.err
}

// fakeRunner is a test double for migration.Runner: it records every
// Run call (the Command it was invoked with, and via rec) and returns
// err, letting tests simulate a failed goose run.
type fakeRunner struct {
	rec *callRecorder
	err error

	calls   int
	lastCmd migration.Command
	lastDir string
}

func (f *fakeRunner) Run(ctx context.Context, cmd migration.Command, migrationsDir string) error {
	f.calls++
	f.lastCmd = cmd
	f.lastDir = migrationsDir
	if f.rec != nil {
		f.rec.record("run")
	}
	return f.err
}

// fakeRoleProvisioner is a test double for migration.RoleProvisioner:
// it records every EnsureAppRole call (the role/password it received,
// and via rec) and returns err.
type fakeRoleProvisioner struct {
	rec *callRecorder
	err error

	calls        int
	lastRole     migration.AppRole
	lastPassword string
}

func (f *fakeRoleProvisioner) EnsureAppRole(ctx context.Context, role migration.AppRole, password string) error {
	f.calls++
	f.lastRole = role
	f.lastPassword = password
	if f.rec != nil {
		f.rec.record("ensure_app_role")
	}
	return f.err
}

func mustCommand(t *testing.T, s string) migration.Command {
	t.Helper()
	cmd, err := migration.ParseCommand(s)
	if err != nil {
		t.Fatalf("migration.ParseCommand(%q) unexpected error: %v", s, err)
	}
	return cmd
}

func mustDatabaseName(t *testing.T, s string) migration.DatabaseName {
	t.Helper()
	name, err := migration.ParseDatabaseName(s)
	if err != nil {
		t.Fatalf("migration.ParseDatabaseName(%q) unexpected error: %v", s, err)
	}
	return name
}

func mustAppRole(t *testing.T, name string, db migration.DatabaseName) migration.AppRole {
	t.Helper()
	role, err := migration.ParseAppRole(name, db)
	if err != nil {
		t.Fatalf("migration.ParseAppRole(%q) unexpected error: %v", name, err)
	}
	return role
}

// TestMigrate_Up_RoleRequested_ProvisionsAfterRunSucceeds is ISSUE-016
// R-c's main wiring path: a "-command up" run with a role requested
// provisions that role exactly once, and only after EnsureExists and
// Run have both already run (the callRecorder's order), matching
// Migrate's documented "GRANT は goose up の後に流す" contract.
func TestMigrate_Up_RoleRequested_ProvisionsAfterRunSucceeds(t *testing.T) {
	rec := &callRecorder{}
	db := &fakeDatabase{rec: rec}
	runner := &fakeRunner{rec: rec}
	prov := &fakeRoleProvisioner{rec: rec}
	svc := New(db, runner, prov)

	dbName := mustDatabaseName(t, "api")
	role := mustAppRole(t, "api_app", dbName)
	roleReq := &RoleRequest{Role: role, Password: "pw"}

	err := svc.Migrate(context.Background(), dbName, mustCommand(t, "up"), "/migrations/api", roleReq)
	if err != nil {
		t.Fatalf("Migrate() unexpected error: %v", err)
	}

	if db.calls != 1 {
		t.Errorf("db.EnsureExists calls = %d, want 1", db.calls)
	}
	if runner.calls != 1 {
		t.Errorf("runner.Run calls = %d, want 1", runner.calls)
	}
	if runner.lastCmd.String() != "up" || runner.lastDir != "/migrations/api" {
		t.Errorf("runner.Run got (cmd=%q, dir=%q), want (up, /migrations/api)", runner.lastCmd, runner.lastDir)
	}
	if prov.calls != 1 {
		t.Fatalf("prov.EnsureAppRole calls = %d, want 1", prov.calls)
	}
	if prov.lastRole != role {
		t.Errorf("prov.EnsureAppRole role = %+v, want %+v", prov.lastRole, role)
	}
	if prov.lastPassword != "pw" {
		t.Errorf("prov.EnsureAppRole password = %q, want %q", prov.lastPassword, "pw")
	}

	wantOrder := []string{"ensure_exists", "run", "ensure_app_role"}
	if !equalStrings(rec.calls, wantOrder) {
		t.Errorf("call order = %v, want %v (role must be provisioned only after the migration run)", rec.calls, wantOrder)
	}
}

// TestMigrate_DownOrStatus_RoleRequested_NeverProvisioned is
// migration.Command.IsUp's gating contract: a role request must be
// silently ignored for "down" and "status", since granting access
// while migrations are being rolled back (or merely inspected) is not
// Migrate's job.
func TestMigrate_DownOrStatus_RoleRequested_NeverProvisioned(t *testing.T) {
	for _, cmdName := range []string{"down", "status"} {
		t.Run(cmdName, func(t *testing.T) {
			db := &fakeDatabase{}
			runner := &fakeRunner{}
			prov := &fakeRoleProvisioner{}
			svc := New(db, runner, prov)

			dbName := mustDatabaseName(t, "api")
			role := mustAppRole(t, "api_app", dbName)
			roleReq := &RoleRequest{Role: role, Password: "pw"}

			err := svc.Migrate(context.Background(), dbName, mustCommand(t, cmdName), "/migrations/api", roleReq)
			if err != nil {
				t.Fatalf("Migrate() unexpected error: %v", err)
			}
			if runner.calls != 1 {
				t.Errorf("runner.Run calls = %d, want 1 (the %s command itself must still run)", runner.calls, cmdName)
			}
			if prov.calls != 0 {
				t.Errorf("prov.EnsureAppRole calls = %d, want 0 for -command %s even though a role was requested", prov.calls, cmdName)
			}
		})
	}
}

// TestMigrate_Up_RoleNotRequested_NeverProvisioned is the backward
// compatibility contract when APP_DB_USER/APP_DB_PASSWORD are both
// unset (cmd/migrator/main.go passes a nil *RoleRequest in that case):
// a nil role must never invoke EnsureAppRole, regardless of command.
func TestMigrate_Up_RoleNotRequested_NeverProvisioned(t *testing.T) {
	db := &fakeDatabase{}
	runner := &fakeRunner{}
	prov := &fakeRoleProvisioner{}
	svc := New(db, runner, prov)

	dbName := mustDatabaseName(t, "api")
	err := svc.Migrate(context.Background(), dbName, mustCommand(t, "up"), "/migrations/api", nil)
	if err != nil {
		t.Fatalf("Migrate() unexpected error: %v", err)
	}
	if runner.calls != 1 {
		t.Errorf("runner.Run calls = %d, want 1", runner.calls)
	}
	if prov.calls != 0 {
		t.Errorf("prov.EnsureAppRole calls = %d, want 0 when role is nil (not requested)", prov.calls)
	}
}

// TestMigrate_Up_RunnerFails_RoleNeverProvisioned is the fail-closed
// ordering guarantee: if the goose run itself fails, EnsureAppRole
// must never be called (granting access on top of a migration that
// did not actually succeed would be worse than skipping it), and
// Migrate must propagate the runner's error.
func TestMigrate_Up_RunnerFails_RoleNeverProvisioned(t *testing.T) {
	rec := &callRecorder{}
	wantErr := errors.New("goose: boom")
	db := &fakeDatabase{rec: rec}
	runner := &fakeRunner{rec: rec, err: wantErr}
	prov := &fakeRoleProvisioner{rec: rec}
	svc := New(db, runner, prov)

	dbName := mustDatabaseName(t, "api")
	role := mustAppRole(t, "api_app", dbName)
	roleReq := &RoleRequest{Role: role, Password: "pw"}

	err := svc.Migrate(context.Background(), dbName, mustCommand(t, "up"), "/migrations/api", roleReq)
	if err == nil {
		t.Fatal("Migrate() = nil error, want the wrapped runner error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("Migrate() error = %v, want it to wrap %v", err, wantErr)
	}
	if prov.calls != 0 {
		t.Errorf("prov.EnsureAppRole calls = %d, want 0 when the migration run fails", prov.calls)
	}

	wantOrder := []string{"ensure_exists", "run"}
	if !equalStrings(rec.calls, wantOrder) {
		t.Errorf("call order = %v, want %v (no ensure_app_role after a failed run)", rec.calls, wantOrder)
	}
}

// TestMigrate_EnsureExistsFails_RunAndRoleNeverCalled asserts the same
// fail-closed ordering one step earlier: if the database bootstrap
// itself fails, neither the migration runner nor the role provisioner
// must be invoked.
func TestMigrate_EnsureExistsFails_RunAndRoleNeverCalled(t *testing.T) {
	wantErr := errors.New("create database: boom")
	db := &fakeDatabase{err: wantErr}
	runner := &fakeRunner{}
	prov := &fakeRoleProvisioner{}
	svc := New(db, runner, prov)

	dbName := mustDatabaseName(t, "api")
	role := mustAppRole(t, "api_app", dbName)
	roleReq := &RoleRequest{Role: role, Password: "pw"}

	err := svc.Migrate(context.Background(), dbName, mustCommand(t, "up"), "/migrations/api", roleReq)
	if err == nil {
		t.Fatal("Migrate() = nil error, want the wrapped EnsureExists error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("Migrate() error = %v, want it to wrap %v", err, wantErr)
	}
	if runner.calls != 0 {
		t.Errorf("runner.Run calls = %d, want 0 when EnsureExists fails", runner.calls)
	}
	if prov.calls != 0 {
		t.Errorf("prov.EnsureAppRole calls = %d, want 0 when EnsureExists fails", prov.calls)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
