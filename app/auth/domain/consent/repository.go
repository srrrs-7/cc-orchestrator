// Package consent models persisted resource-owner grants for OAuth scopes.
package consent

import "context"

// Repository persists which scopes a user granted to a client.
type Repository interface {
	HasGrant(ctx context.Context, userID, clientID, scope string) (bool, error)
	SaveGrant(ctx context.Context, userID, clientID, scope string) error
}
