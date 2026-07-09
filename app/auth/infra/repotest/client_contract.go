// Package repotest provides shared "behavioral contract" test suites
// for app/auth's three persisted aggregates (client, user, authcode;
// SPEC-005 R2). Each domain package's Repository interface
// (domain/<aggregate>/repository.go) is the single source of truth
// for what an implementation must do; the Run*RepositoryContract
// functions in this package exercise those contracts once, so
// infra/memory and (once implemented) infra/postgres can both be
// proven to behave identically without duplicating test logic
// per-implementation.
//
// These files carry no build tag: they must compile and be usable
// both by the default (untagged) build -- exercised today against
// infra/memory -- and by the "integration" build (see
// infra/postgres/*_integration_test.go) once infra/postgres exists.
// They must therefore not depend on anything beyond the standard
// library and the relevant domain package.
package repotest

import (
	"context"
	"errors"
	"testing"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/client"
)

// NewClientRepository seeds exactly the given clients into a fresh
// (empty) store and returns it as a client.Repository.
//
// client.Repository is read-only (FindByID only; see
// domain/client/repository.go): seeding is necessarily performed
// outside the interface, through whatever mechanism each
// implementation uses in production (infra/memory.ClientRepository's
// Seed method; infra/postgres's planned startup idempotent-seed
// UpsertClient, per docs/plans/SPEC-005-plan.md §2.2). Implementations
// MUST start from an empty store on every call, the same way
// memory.NewClientRepository() does, so subtests never observe data
// left behind by another subtest.
type NewClientRepository func(t *testing.T, seed ...*client.Client) client.Repository

// RunClientRepositoryContract runs the behavioral contract shared by
// every client.Repository implementation (SPEC-005 R2): FindByID
// round-trips a seeded Client (including its multi-valued attributes
// -- redirect_uris / allowed_scopes / response_types / grant_types),
// and reports client.ErrNotFound for an id that was never seeded.
func RunClientRepositoryContract(t *testing.T, newRepo NewClientRepository) {
	t.Helper()

	t.Run("FindByID finds a seeded client with every field round-tripped", func(t *testing.T) {
		c := newTestClient(t, "demo-client",
			[]string{"http://localhost:3000/callback", "http://localhost:3000/other-callback"},
			[]string{"openid", "profile"},
			[]string{"code"},
			[]string{"authorization_code"},
		)
		repo := newRepo(t, c)

		got, err := repo.FindByID(context.Background(), c.ID())
		if err != nil {
			t.Fatalf("FindByID() unexpected error: %v", err)
		}
		assertSameClient(t, got, c)
	})

	t.Run("FindByID for an id that was never seeded returns ErrNotFound", func(t *testing.T) {
		repo := newRepo(t) // no seed data at all
		unknownID, err := client.ParseClientID("does-not-exist")
		if err != nil {
			t.Fatalf("setup ParseClientID() unexpected error: %v", err)
		}

		_, err = repo.FindByID(context.Background(), unknownID)
		if !errors.Is(err, client.ErrNotFound) {
			t.Fatalf("FindByID() error = %v, want wrapping %v", err, client.ErrNotFound)
		}
	})

	t.Run("FindByID round-trips a client with empty scope/response_type/grant_type lists", func(t *testing.T) {
		c := newTestClient(t, "bare-client",
			[]string{"http://localhost:4000/callback"},
			nil, nil, nil,
		)
		repo := newRepo(t, c)

		got, err := repo.FindByID(context.Background(), c.ID())
		if err != nil {
			t.Fatalf("FindByID() unexpected error: %v", err)
		}
		if len(got.AllowedScopes()) != 0 {
			t.Errorf("AllowedScopes() = %v, want empty", got.AllowedScopes())
		}
		if len(got.ResponseTypes()) != 0 {
			t.Errorf("ResponseTypes() = %v, want empty", got.ResponseTypes())
		}
		if len(got.GrantTypes()) != 0 {
			t.Errorf("GrantTypes() = %v, want empty", got.GrantTypes())
		}
	})

	// SPEC-005 plan §5.3's boundary row for client ("空 scope/redirect の
	// 配列") lists redirect_uris alongside scope/response_type/grant_type;
	// the subtest above only exercises the latter three. RedirectURIs is
	// backed by a real (non-set) []client.RedirectURI slice in
	// infra/memory and a jsonb array in infra/postgres, so an empty list
	// exercises a distinct code path (in particular jsonb NOT NULL
	// encoding an empty Go slice as "[]", never SQL NULL -- see
	// infra/postgres/client_repository.go's encodeStringSlice).
	t.Run("FindByID round-trips a client with an empty redirect_uris list", func(t *testing.T) {
		c := newTestClient(t, "no-redirect-client", nil, []string{"openid"}, []string{"code"}, []string{"authorization_code"})
		repo := newRepo(t, c)

		got, err := repo.FindByID(context.Background(), c.ID())
		if err != nil {
			t.Fatalf("FindByID() unexpected error: %v", err)
		}
		if len(got.RedirectURIs()) != 0 {
			t.Errorf("RedirectURIs() = %v, want empty", got.RedirectURIs())
		}
	})
}

func newTestClient(t *testing.T, id string, redirectURIs, scopes, responseTypes, grantTypes []string) *client.Client {
	t.Helper()

	clientID, err := client.ParseClientID(id)
	if err != nil {
		t.Fatalf("setup ParseClientID(%q) unexpected error: %v", id, err)
	}

	uris := make([]client.RedirectURI, 0, len(redirectURIs))
	for _, s := range redirectURIs {
		uri, err := client.NewRedirectURI(s)
		if err != nil {
			t.Fatalf("setup NewRedirectURI(%q) unexpected error: %v", s, err)
		}
		uris = append(uris, uri)
	}

	return client.New(clientID, uris, scopes, responseTypes, grantTypes)
}

// assertSameClient compares every observable field of got against
// want. Multi-valued attributes are compared as sets (order-
// independent): AllowedScopes/ResponseTypes/GrantTypes are backed by
// a map internally even in infra/memory (client.Client.fromSet
// iterates a Go map), so no implementation -- memory or Postgres --
// is expected to promise a stable order. RedirectURIs is backed by a
// real slice in infra/memory, but a jsonb round trip through Postgres
// is not guaranteed to preserve order either, so it is compared the
// same (order-independent) way here to avoid over-constraining a
// property the domain does not itself promise.
func assertSameClient(t *testing.T, got, want *client.Client) {
	t.Helper()
	if got.ID() != want.ID() {
		t.Errorf("ID() = %v, want %v", got.ID(), want.ID())
	}
	assertSameRedirectURIs(t, got.RedirectURIs(), want.RedirectURIs())
	assertSameStringSet(t, "AllowedScopes", got.AllowedScopes(), want.AllowedScopes())
	assertSameStringSet(t, "ResponseTypes", got.ResponseTypes(), want.ResponseTypes())
	assertSameStringSet(t, "GrantTypes", got.GrantTypes(), want.GrantTypes())
}

func assertSameRedirectURIs(t *testing.T, got, want []client.RedirectURI) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("RedirectURIs() = %v (len %d), want %v (len %d)", got, len(got), want, len(want))
		return
	}
	for _, w := range want {
		found := false
		for _, g := range got {
			if g.Equal(w) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("RedirectURIs() = %v, missing %v", got, w)
		}
	}
}

func assertSameStringSet(t *testing.T, field string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s() = %v (len %d), want %v (len %d)", field, got, len(got), want, len(want))
		return
	}
	wantSet := make(map[string]bool, len(want))
	for _, w := range want {
		wantSet[w] = true
	}
	for _, g := range got {
		if !wantSet[g] {
			t.Errorf("%s() = %v, want %v (unexpected value %q)", field, got, want, g)
		}
	}
}
