// env_test.go exercises env.go's Env / NewEnv / (Env).validate /
// (Env).writerConfig / (Env).readerConfig -- the sole place app/api
// reads the process environment (os.Getenv), consolidated here by the
// env.go refactor. All os.Getenv reads happen inside NewEnv, so these
// tests use t.Setenv (auto-restored per test, which also neutralizes
// any ambient value already present in the CI/dev environment) to
// isolate each case, and never require a live Postgres: (Env).validate
// only checks presence of DB_* fields via postgres.Config.Validate,
// it never dials a connection (SPEC-011: Postgres is the only backend;
// fail-closed is enforced by Config.Validate).
package main

import (
	"strings"
	"testing"
)

// envVars lists every environment variable NewEnv reads.
var envVars = []string{
	"PORT",
	"DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD", "DB_SSLMODE",
	"DB_READER_HOST", "DB_READER_PORT", "DB_READER_NAME", "DB_READER_USER", "DB_READER_PASSWORD", "DB_READER_SSLMODE",
}

// readerFallbackVars lists just the DB_READER_* variables SPEC-010
// adds (a subset of envVars above), for tests that only need to reset/
// set the reader side.
var readerFallbackVars = []string{
	"DB_READER_HOST", "DB_READER_PORT", "DB_READER_NAME",
	"DB_READER_USER", "DB_READER_PASSWORD", "DB_READER_SSLMODE",
}

// TestNewEnv_Defaults confirms PORT/DB_PORT/DB_SSLMODE fall back to
// their documented defaults when every relevant variable is unset,
// while variables without a default (DB_HOST, DB_NAME, DB_USER,
// DB_PASSWORD) stay empty. DB_SSLMODE's default is fail-closed
// ("require", ISSUE-016 m-2): omitting it must not silently downgrade
// the connection to plaintext.
func TestNewEnv_Defaults(t *testing.T) {
	for _, key := range envVars {
		t.Setenv(key, "")
	}

	e := NewEnv()

	if e.Port != "8080" {
		t.Errorf("Env.Port = %q, want %q", e.Port, "8080")
	}
	if e.DBPort != "5432" {
		t.Errorf("Env.DBPort = %q, want %q", e.DBPort, "5432")
	}
	if e.DBSSLMode != "require" {
		t.Errorf("Env.DBSSLMode = %q, want %q", e.DBSSLMode, "require")
	}
	for _, tc := range []struct {
		name string
		got  string
	}{
		{"DBHost", e.DBHost},
		{"DBName", e.DBName},
		{"DBUser", e.DBUser},
		{"DBPassword", e.DBPassword},
	} {
		if tc.got != "" {
			t.Errorf("Env.%s = %q, want empty (no default)", tc.name, tc.got)
		}
	}

	// SPEC-010 R3/R4: with every DB_READER_* also unset, Env.DBReader
	// must fall back to the writer's own (already-defaulted) values
	// field-for-field -- including the writer's own defaults (DBPort's
	// "5432", DBSSLMode's fail-closed "require"), not just its unset
	// ("") fields.
	readerCases := []struct {
		name string
		got  string
		want string
	}{
		{"DBReader.Host", e.DBReader.Host, e.DBHost},
		{"DBReader.Port", e.DBReader.Port, e.DBPort},
		{"DBReader.Name", e.DBReader.Name, e.DBName},
		{"DBReader.User", e.DBReader.User, e.DBUser},
		{"DBReader.Password", e.DBReader.Password, e.DBPassword},
		{"DBReader.SSLMode", e.DBReader.SSLMode, e.DBSSLMode},
	}
	for _, tc := range readerCases {
		if tc.got != tc.want {
			t.Errorf("Env.%s = %q, want it to fall back to %q (the writer's own value)", tc.name, tc.got, tc.want)
		}
	}
}

// TestNewEnv_ReadsEveryVar sets every variable NewEnv reads to a
// distinct value and asserts each is threaded through to the
// corresponding Env field unchanged. This migrates the env-read
// coverage the pre-refactor infra/postgres.ConfigFromEnv tests used to
// carry for the DB_* subset, now consolidated with the non-DB
// variables under NewEnv.
func TestNewEnv_ReadsEveryVar(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("DB_HOST", "db.internal")
	t.Setenv("DB_PORT", "6543")
	t.Setenv("DB_NAME", "appdb")
	t.Setenv("DB_USER", "appuser")
	t.Setenv("DB_PASSWORD", "s3cret-pw")
	t.Setenv("DB_SSLMODE", "require")

	e := NewEnv()

	cases := []struct {
		field string
		got   string
		want  string
	}{
		{"Port", e.Port, "9090"},
		{"DBHost", e.DBHost, "db.internal"},
		{"DBPort", e.DBPort, "6543"},
		{"DBName", e.DBName, "appdb"},
		{"DBUser", e.DBUser, "appuser"},
		{"DBPassword", e.DBPassword, "s3cret-pw"},
		{"DBSSLMode", e.DBSSLMode, "require"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("Env.%s = %q, want %q", tc.field, tc.got, tc.want)
		}
	}
}

// TestValidate_PostgresMode_RequiresDBFields is the 異常系 case: when
// DB_HOST is set (selecting Postgres mode), the remaining required
// DB_* fields (DB_NAME/DB_USER/DB_PASSWORD) become mandatory, and
// validate's error must name every one that is missing.
func TestValidate_PostgresMode_RequiresDBFields(t *testing.T) {
	e := Env{DBHost: "db.internal"} // DBName/DBUser/DBPassword deliberately empty

	err := e.validate()
	if err == nil {
		t.Fatal("validate() = nil error, want an error naming the missing DB_NAME/DB_USER/DB_PASSWORD")
	}
	for _, want := range []string{"DB_NAME", "DB_USER", "DB_PASSWORD"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("validate() error = %q, want it to name missing variable %s", err.Error(), want)
		}
	}
}

// TestValidate_PostgresMode_ErrorNeverLeaksPassword is the security
// invariant: even though DBPassword is populated (unlike DBName/DBUser
// alongside it), validate's error must never echo its value -- only
// missing variable *names* may appear.
func TestValidate_PostgresMode_ErrorNeverLeaksPassword(t *testing.T) {
	const secret = "sup3r-secret-xyz"
	e := Env{DBHost: "db.internal", DBPassword: secret} // DBName/DBUser deliberately empty

	err := e.validate()
	if err == nil {
		t.Fatal("validate() = nil error, want an error (DB_NAME/DB_USER missing)")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("validate() error %q leaks the DB_PASSWORD value", err.Error())
	}
	for _, want := range []string{"DB_NAME", "DB_USER"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("validate() error = %q, want it to name missing variable %s", err.Error(), want)
		}
	}
}

// TestValidate_FailClosed is the 異常系 fail-closed case: with every
// field at its zero value (DB_HOST unset), validate must not succeed
// -- it must return an error mentioning DB_HOST (the first required
// field that is missing). SPEC-011 removes memory fallback; Postgres
// is always required and Config.Validate enforces it.
func TestValidate_FailClosed(t *testing.T) {
	e := Env{}

	err := e.validate()
	if err == nil {
		t.Fatal("validate() = nil error, want a fail-closed error (DB_HOST unset)")
	}
	if !strings.Contains(err.Error(), "DB_HOST") {
		t.Errorf("validate() error = %q, want it to mention DB_HOST", err.Error())
	}
}

// TestValidate_PostgresMode_AllFieldsPresent is the 正常系 case: a
// fully-populated Postgres Env succeeds with no error.
func TestValidate_PostgresMode_AllFieldsPresent(t *testing.T) {
	e := Env{DBHost: "db.internal", DBName: "appdb", DBUser: "appuser", DBPassword: "pw"}
	// SPEC-010: validate() also validates the reader Config in Postgres
	// mode. A hand-built Env literal (unlike one NewEnv returns) does
	// not get DB_READER_* fallback for free -- that fallback happens
	// inside NewEnv itself -- so DBReader is populated here to mirror
	// what NewEnv's fallback would produce when every DB_READER_* is
	// unset (reader == writer), keeping this test's original intent
	// (a fully-populated Postgres Env succeeds) isolated from the
	// separate fallback behavior TestNewEnv_ReaderFallback_* below
	// exercises directly.
	e.DBReader.Host = e.DBHost
	e.DBReader.Name = e.DBName
	e.DBReader.User = e.DBUser
	e.DBReader.Password = e.DBPassword

	if err := e.validate(); err != nil {
		t.Fatalf("validate() unexpected error: %v", err)
	}
}

// --- SPEC-010: DB_READER_* fallback (R3/R4) ---------------------------
//
// cmd/api/env.go additionally reads DB_READER_HOST/PORT/NAME/USER/
// PASSWORD/SSLMODE into Env.DBReader (docs/plans/SPEC-010-plan.md's
// "変更ファイル" table for cmd/api/env.go), falling each unset item
// back to the corresponding writer DB_* value inside NewEnv itself --
// so by the time NewEnv returns, Env.DBReader already holds the
// *effective* reader values, not the raw (possibly empty) DB_READER_*
// strings. (Env).writerConfig() / (Env).readerConfig() project Env's
// writer fields / Env.DBReader into a postgres.Config each, replacing
// the pre-SPEC-010 (Env).dbConfig() (which only ever needed to build
// one Config).
//
// These two method names and the Env.DBReader field are the exact
// symbols docs/plans/SPEC-010-plan.md's implementation table names for
// cmd/api/env.go; impl-api must implement them under these names for
// this file to compile (see this Spec's tester report for the full
// list of symbols pinned across cmd/api and cmd/authz).

// TestNewEnv_ReaderFallback_AllUnset covers R3/R4's core fallback case:
// with every DB_READER_* variable unset, Env.DBReader must equal the
// writer's own DB_* values field-for-field, and (Env).readerConfig()
// must equal (Env).writerConfig() -- the equality postgres.OpenPair
// relies on to share a single pool instead of opening a second one
// (non-functional requirement: 二重に開かない).
func TestNewEnv_ReaderFallback_AllUnset(t *testing.T) {
	for _, key := range readerFallbackVars {
		t.Setenv(key, "")
	}
	t.Setenv("DB_HOST", "writer.internal")
	t.Setenv("DB_PORT", "6543")
	t.Setenv("DB_NAME", "appdb")
	t.Setenv("DB_USER", "appuser")
	t.Setenv("DB_PASSWORD", "s3cret-pw")
	t.Setenv("DB_SSLMODE", "require")

	e := NewEnv()

	cases := []struct {
		name       string
		gotReader  string
		wantWriter string
	}{
		{"Host", e.DBReader.Host, e.DBHost},
		{"Port", e.DBReader.Port, e.DBPort},
		{"Name", e.DBReader.Name, e.DBName},
		{"User", e.DBReader.User, e.DBUser},
		{"Password", e.DBReader.Password, e.DBPassword},
		{"SSLMode", e.DBReader.SSLMode, e.DBSSLMode},
	}
	for _, tc := range cases {
		if tc.gotReader != tc.wantWriter {
			t.Errorf("Env.DBReader.%s = %q, want it to fall back to writer value %q", tc.name, tc.gotReader, tc.wantWriter)
		}
	}

	if e.readerConfig() != e.writerConfig() {
		t.Errorf("readerConfig() = %+v, want it to equal writerConfig() = %+v when every DB_READER_* is unset", e.readerConfig(), e.writerConfig())
	}
}

// TestNewEnv_ReaderFallback_HostOnly covers R4's per-item fallback with
// exactly one override: DB_READER_HOST diverges from the writer while
// every other DB_READER_* stays unset and therefore still falls back.
func TestNewEnv_ReaderFallback_HostOnly(t *testing.T) {
	for _, key := range readerFallbackVars {
		t.Setenv(key, "")
	}
	t.Setenv("DB_HOST", "writer.internal")
	t.Setenv("DB_PORT", "6543")
	t.Setenv("DB_NAME", "appdb")
	t.Setenv("DB_USER", "appuser")
	t.Setenv("DB_PASSWORD", "s3cret-pw")
	t.Setenv("DB_SSLMODE", "require")
	t.Setenv("DB_READER_HOST", "replica.internal")

	e := NewEnv()

	if e.DBReader.Host != "replica.internal" {
		t.Errorf("Env.DBReader.Host = %q, want %q (explicit override)", e.DBReader.Host, "replica.internal")
	}
	cases := []struct {
		name       string
		gotReader  string
		wantWriter string
	}{
		{"Port", e.DBReader.Port, e.DBPort},
		{"Name", e.DBReader.Name, e.DBName},
		{"User", e.DBReader.User, e.DBUser},
		{"Password", e.DBReader.Password, e.DBPassword},
		{"SSLMode", e.DBReader.SSLMode, e.DBSSLMode},
	}
	for _, tc := range cases {
		if tc.gotReader != tc.wantWriter {
			t.Errorf("Env.DBReader.%s = %q, want it to still fall back to writer value %q", tc.name, tc.gotReader, tc.wantWriter)
		}
	}

	if got, want := e.readerConfig().Host, "replica.internal"; got != want {
		t.Errorf("readerConfig().Host = %q, want %q", got, want)
	}
	if e.readerConfig() == e.writerConfig() {
		t.Error("readerConfig() == writerConfig(), want them to differ once DB_READER_HOST diverges (OpenPair must open a second pool)")
	}
}

// TestNewEnv_ReaderFallback_PerField covers R4's remaining per-item
// overrides (Port/Name/User/Password/SSLMode): setting exactly one
// DB_READER_X must change only that field on Env.DBReader, leaving
// every other field to fall back to its writer counterpart.
func TestNewEnv_ReaderFallback_PerField(t *testing.T) {
	writer := map[string]string{
		"DB_HOST":     "writer.internal",
		"DB_PORT":     "6543",
		"DB_NAME":     "appdb",
		"DB_USER":     "appuser",
		"DB_PASSWORD": "s3cret-pw",
		"DB_SSLMODE":  "require",
	}

	tests := []struct {
		readerVar string
		override  string
		fieldName string
		get       func(Env) string
	}{
		{"DB_READER_PORT", "6544", "Port", func(e Env) string { return e.DBReader.Port }},
		{"DB_READER_NAME", "replicadb", "Name", func(e Env) string { return e.DBReader.Name }},
		{"DB_READER_USER", "replicauser", "User", func(e Env) string { return e.DBReader.User }},
		{"DB_READER_PASSWORD", "replica-pw", "Password", func(e Env) string { return e.DBReader.Password }},
		{"DB_READER_SSLMODE", "disable", "SSLMode", func(e Env) string { return e.DBReader.SSLMode }},
	}

	for _, tt := range tests {
		t.Run(tt.fieldName, func(t *testing.T) {
			for _, key := range readerFallbackVars {
				t.Setenv(key, "")
			}
			for k, v := range writer {
				t.Setenv(k, v)
			}
			t.Setenv(tt.readerVar, tt.override)

			e := NewEnv()

			if got := tt.get(e); got != tt.override {
				t.Errorf("Env.DBReader.%s = %q, want override %q", tt.fieldName, got, tt.override)
			}

			all := []struct {
				name       string
				gotReader  string
				wantWriter string
			}{
				{"Host", e.DBReader.Host, e.DBHost},
				{"Port", e.DBReader.Port, e.DBPort},
				{"Name", e.DBReader.Name, e.DBName},
				{"User", e.DBReader.User, e.DBUser},
				{"Password", e.DBReader.Password, e.DBPassword},
				{"SSLMode", e.DBReader.SSLMode, e.DBSSLMode},
			}
			for _, tc := range all {
				if tc.name == tt.fieldName {
					continue // this is the field under test, already asserted above
				}
				if tc.gotReader != tc.wantWriter {
					t.Errorf("Env.DBReader.%s = %q, want it to fall back to writer value %q", tc.name, tc.gotReader, tc.wantWriter)
				}
			}
		})
	}
}

// TestValidate_PostgresMode_WithReaderOverride_StaysValid exercises the
// full NewEnv -> validate() path with the writer fully valid and only
// DB_READER_HOST overridden: validate() must still return nil, since
// every other DB_READER_* field falls back to the (already valid)
// writer value (docs/plans/SPEC-010-plan.md: "validate() は Postgres
// モード時に writer/reader 双方の Config.Validate()(reader は fallback
// 済で writer 妥当なら妥当)").
func TestValidate_PostgresMode_WithReaderOverride_StaysValid(t *testing.T) {
	for _, key := range readerFallbackVars {
		t.Setenv(key, "")
	}
	t.Setenv("DB_HOST", "db.internal")
	t.Setenv("DB_NAME", "appdb")
	t.Setenv("DB_USER", "appuser")
	t.Setenv("DB_PASSWORD", "pw")
	t.Setenv("DB_READER_HOST", "replica.internal")

	e := NewEnv()
	if err := e.validate(); err != nil {
		t.Fatalf("validate() unexpected error: %v", err)
	}
	if e.readerConfig().Host != "replica.internal" {
		t.Errorf("readerConfig().Host = %q, want %q", e.readerConfig().Host, "replica.internal")
	}
}
