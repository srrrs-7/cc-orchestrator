package token

// KeyEntry describes a single key in the key ring.
// Exactly one of PrivateKeyPEM (active signing key) or
// PublicKeyPEM (retired verify-only key) must be non-empty.
// Domain types use PEM strings rather than crypto primitives so
// that this package stays free of crypto/rsa imports.
type KeyEntry struct {
	Kid           string
	PrivateKeyPEM string // non-empty for the active signing key
	PublicKeyPEM  string // non-empty for retired verify-only keys
}

// SigningMaterial is the loaded key ring returned by KeyRingLoader.
// It carries the active key ID and the full set of key entries
// (active + any retired verify-only keys for rotation overlap).
type SigningMaterial struct {
	ActiveKid string
	Keys      []KeyEntry
}

// KeyRingLoader is a port (domain-declared interface) for loading the
// signing key ring at startup. The two concrete implementations are:
//
//   - infra/jwt.FileLoader  — reads from SIGNING_KEYS_FILE (production)
//   - infra/jwt.EphemeralLoader — generates a fresh RSA key in memory
//     (development / test, SIGNING_KEYS_FILE unset)
type KeyRingLoader interface {
	Load() (SigningMaterial, error)
}
