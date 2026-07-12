package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/authcode"
	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/refreshtoken"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres"
	"github.com/srrrs-7/cc-orchestrator/app/auth/infra/postgres/testsupport"
)

// TestAuthCodeRepository_PurgeExpired verifies that PurgeExpired deletes
// only rows whose expires_at is in the past, leaves live rows intact, and
// returns the correct affected-row count (ISSUE-015).
func TestAuthCodeRepository_PurgeExpired(t *testing.T) {
	db := testsupport.OpenTestDB(t)
	testsupport.TruncateTable(t, db, "authorization_codes")
	repo := postgres.NewAuthCodeRepository(db)
	ctx := t.Context()

	challenge, err := authcode.NewCodeChallenge(
		"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
		authcode.CodeChallengeMethodS256,
	)
	if err != nil {
		t.Fatalf("NewCodeChallenge: %v", err)
	}

	// Insert a live code (expires 1 hour from now).
	liveCode := mustParseCode(t, "live-code-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	liveAC := authcode.Reconstruct(
		liveCode,
		authcode.NewClientID("client-1"),
		authcode.NewUserID("user-1"),
		authcode.NewRedirectURI("http://localhost:3000/callback"),
		mustParseScope(t, "openid"),
		authcode.NewNonce(""),
		challenge,
		time.Time{},
		time.Now().Add(1*time.Hour),
		false,
	)
	if err := repo.Save(ctx, liveAC); err != nil {
		t.Fatalf("Save live code: %v", err)
	}

	// Insert an expired code (expired 1 second ago) directly so we do not
	// have to sleep: use raw SQL to bypass the domain constructor's future-
	// expiry guard.
	insertExpiredAuthCode(t, db, "expired-code-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	// Baseline: 1 expired row exists.
	t.Run("purges only expired rows", func(t *testing.T) {
		n, err := repo.PurgeExpired(ctx)
		if err != nil {
			t.Fatalf("PurgeExpired: %v", err)
		}
		if n != 1 {
			t.Errorf("PurgeExpired returned %d deleted; want 1", n)
		}

		// Live code must still be findable.
		if _, err := repo.FindByCode(ctx, liveCode); err != nil {
			t.Errorf("FindByCode(live) after purge: %v; want nil", err)
		}

		// Expired code must be gone (FindByCode should return ErrNotFound).
		expiredCode := mustParseCode(t, "expired-code-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		_, err = repo.FindByCode(ctx, expiredCode)
		if !isNotFound(err) {
			t.Errorf("FindByCode(expired) after purge: %v; want ErrNotFound", err)
		}
	})

	t.Run("no-op when nothing expired", func(t *testing.T) {
		n, err := repo.PurgeExpired(ctx)
		if err != nil {
			t.Fatalf("PurgeExpired (second call): %v", err)
		}
		if n != 0 {
			t.Errorf("PurgeExpired (second call) returned %d; want 0", n)
		}
	})
}

// TestRefreshTokenRepository_PurgeExpired verifies that PurgeExpired deletes
// ALL expired rows (both consumed and unconsumed) and returns the correct
// count, leaving live rows intact (ISSUE-019 item 1).
func TestRefreshTokenRepository_PurgeExpired(t *testing.T) {
	db := testsupport.OpenTestDB(t)
	testsupport.TruncateTable(t, db, "refresh_tokens")
	repo := postgres.NewRefreshTokenRepository(db)
	ctx := t.Context()

	// Insert a live (unconsumed, unexpired) refresh token.
	liveHash := mustParseTokenHash(t, "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899")
	liveRT := refreshtoken.Reconstruct(
		liveHash,
		mustParseFamilyID(t, "fam-live-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		refreshtoken.NewClientID("client-1"),
		refreshtoken.NewUserID("user-1"),
		mustParseRTScope(t, "openid"),
		time.Time{},
		time.Now().Add(24*time.Hour),
		false,
	)
	if err := repo.Save(ctx, liveRT); err != nil {
		t.Fatalf("Save live refresh token: %v", err)
	}

	// Insert an expired+unconsumed row directly.
	insertExpiredRefreshToken(t, db, "dead00000000000000000000000000000000000000000000000000000000dead",
		"fam-expired-1-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false)

	// Insert an expired+consumed row (rotation happened, TTL elapsed): this
	// is the key scenario for ISSUE-019 item 1 -- consumed rows that reuse-
	// detection no longer needs (because the TTL has passed) must be GCed.
	insertExpiredRefreshToken(t, db, "c0ffee00000000000000000000000000000000000000000000000000c0ffee00",
		"fam-expired-2-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true)

	t.Run("purges expired rows (consumed and unconsumed)", func(t *testing.T) {
		n, err := repo.PurgeExpired(ctx)
		if err != nil {
			t.Fatalf("PurgeExpired: %v", err)
		}
		if n != 2 {
			t.Errorf("PurgeExpired returned %d deleted; want 2", n)
		}

		// Live token must still be findable.
		if _, err := repo.FindByTokenHash(ctx, liveHash); err != nil {
			t.Errorf("FindByTokenHash(live) after purge: %v; want nil", err)
		}

		// Expired tokens must be gone.
		for _, hashStr := range []string{
			"dead00000000000000000000000000000000000000000000000000000000dead",
			"c0ffee00000000000000000000000000000000000000000000000000c0ffee00",
		} {
			h := mustParseTokenHash(t, hashStr)
			_, err := repo.FindByTokenHash(ctx, h)
			if !isNotFound(err) {
				t.Errorf("FindByTokenHash(%s) after purge: %v; want ErrNotFound", hashStr[:8], err)
			}
		}
	})

	t.Run("no-op when nothing expired", func(t *testing.T) {
		n, err := repo.PurgeExpired(ctx)
		if err != nil {
			t.Fatalf("PurgeExpired (second call): %v", err)
		}
		if n != 0 {
			t.Errorf("PurgeExpired (second call) returned %d; want 0", n)
		}
	})
}

// insertExpiredAuthCode inserts a row into authorization_codes with an
// expires_at in the past using raw SQL so we can bypass the domain
// constructor's future-expiry guard.
func insertExpiredAuthCode(t *testing.T, db *sql.DB, code string) {
	t.Helper()
	const query = `INSERT INTO authorization_codes
		(code, client_id, user_id, redirect_uri, scope, challenge, challenge_method, expires_at, consumed)
		VALUES ($1, 'client-1', 'user-1', 'http://localhost:3000/callback', 'openid',
		        'E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM', 'S256',
		        now() - interval '1 second', false)`
	if _, err := db.ExecContext(context.Background(), query, code); err != nil {
		t.Fatalf("insertExpiredAuthCode(%s): %v", code, err)
	}
}

// insertExpiredRefreshToken inserts a row into refresh_tokens with an
// expires_at in the past using raw SQL.
func insertExpiredRefreshToken(t *testing.T, db *sql.DB, tokenHash, familyID string, consumed bool) {
	t.Helper()
	const query = `INSERT INTO refresh_tokens
		(token_hash, family_id, client_id, user_id, scope, expires_at, consumed)
		VALUES ($1, $2, 'client-1', 'user-1', 'openid',
		        now() - interval '1 second', $3)`
	if _, err := db.ExecContext(context.Background(), query, tokenHash, familyID, consumed); err != nil {
		t.Fatalf("insertExpiredRefreshToken(%s): %v", tokenHash[:8], err)
	}
}

// isNotFound reports whether err wraps one of the domain ErrNotFound
// sentinels (either authcode or refreshtoken variant).
func isNotFound(err error) bool {
	return errors.Is(err, authcode.ErrNotFound) ||
		errors.Is(err, refreshtoken.ErrNotFound)
}

// helpers ------------------------------------------------------------------

func mustParseCode(t *testing.T, s string) authcode.Code {
	t.Helper()
	c, err := authcode.ParseCode(s)
	if err != nil {
		t.Fatalf("ParseCode(%q): %v", s, err)
	}
	return c
}

func mustParseScope(t *testing.T, s string) authcode.Scope {
	t.Helper()
	sc, err := authcode.ParseScope(s)
	if err != nil {
		t.Fatalf("ParseScope(%q): %v", s, err)
	}
	return sc
}

func mustParseTokenHash(t *testing.T, s string) refreshtoken.TokenHash {
	t.Helper()
	h, err := refreshtoken.ParseTokenHash(s)
	if err != nil {
		t.Fatalf("ParseTokenHash(%q): %v", s, err)
	}
	return h
}

func mustParseFamilyID(t *testing.T, s string) refreshtoken.FamilyID {
	t.Helper()
	f, err := refreshtoken.ParseFamilyID(s)
	if err != nil {
		t.Fatalf("ParseFamilyID(%q): %v", s, err)
	}
	return f
}

func mustParseRTScope(t *testing.T, s string) refreshtoken.Scope {
	t.Helper()
	sc, err := refreshtoken.ParseScope(s)
	if err != nil {
		t.Fatalf("ParseScope(%q): %v", s, err)
	}
	return sc
}
