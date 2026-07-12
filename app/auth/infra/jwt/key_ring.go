package jwt

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"

	"github.com/srrrs-7/cc-orchestrator/app/auth/domain/token"
)

// KeyRing groups the token.Signer, token.Verifier, and
// token.KeyProvider derived from a loaded SigningMaterial.
// Use NewKeyRingFromMaterial to build one.
type KeyRing struct {
	signer      *Signer
	verifier    *MultiKeyVerifier
	keyProvider *MultiKeyProvider
}

// Signer returns the active token.Signer (signs with the active key).
func (k *KeyRing) Signer() token.Signer { return k.signer }

// Verifier returns the multi-key token.Verifier (verifies by kid).
func (k *KeyRing) Verifier() token.Verifier { return k.verifier }

// KeyProvider returns the multi-key token.KeyProvider (publishes all
// public keys in the JWKS endpoint).
func (k *KeyRing) KeyProvider() token.KeyProvider { return k.keyProvider }

// NewKeyRingFromMaterial parses a SigningMaterial into a KeyRing. It
// requires that material.ActiveKid has a corresponding private key
// entry; all other entries may carry only a public key (retired keys).
func NewKeyRingFromMaterial(material token.SigningMaterial) (*KeyRing, error) {
	publicKeys := make(map[string]*rsa.PublicKey, len(material.Keys))
	var activeSigner *Signer

	for _, entry := range material.Keys {
		switch {
		case entry.PrivateKeyPEM != "":
			priv, err := parseRSAPrivateKey(entry.PrivateKeyPEM)
			if err != nil {
				return nil, fmt.Errorf("jwt: key ring: parse private key for kid %q: %w", entry.Kid, err)
			}
			publicKeys[entry.Kid] = &priv.PublicKey
			if entry.Kid == material.ActiveKid {
				activeSigner = NewSigner(priv, entry.Kid)
			}
		case entry.PublicKeyPEM != "":
			pub, err := parseRSAPublicKey(entry.PublicKeyPEM)
			if err != nil {
				return nil, fmt.Errorf("jwt: key ring: parse public key for kid %q: %w", entry.Kid, err)
			}
			publicKeys[entry.Kid] = pub
		default:
			return nil, fmt.Errorf("jwt: key ring: kid %q has neither private_key_pem nor public_key_pem", entry.Kid)
		}
	}

	if activeSigner == nil {
		return nil, fmt.Errorf("jwt: key ring: no private key found for active_kid %q", material.ActiveKid)
	}

	return &KeyRing{
		signer:      activeSigner,
		verifier:    NewMultiKeyVerifier(publicKeys),
		keyProvider: NewMultiKeyProvider(material.ActiveKid, publicKeys),
	}, nil
}

// MultiKeyVerifier implements token.Verifier with support for multiple
// public keys. It selects the verification key from the JWT header's
// "kid" field, allowing tokens issued under different (e.g. rotated)
// signing keys to all be verified during the rotation overlap period.
type MultiKeyVerifier struct {
	keys map[string]*rsa.PublicKey // kid → public key
}

var _ token.Verifier = (*MultiKeyVerifier)(nil)

// NewMultiKeyVerifier builds a MultiKeyVerifier from a kid → public key map.
func NewMultiKeyVerifier(keys map[string]*rsa.PublicKey) *MultiKeyVerifier {
	return &MultiKeyVerifier{keys: keys}
}

// Verify parses the compact JWT, requires alg=RS256 (algorithm-confusion
// defense), selects the public key by kid from the header, and delegates
// to the standard RS256/SHA-256 verification path.
func (m *MultiKeyVerifier) Verify(tokenString string) (token.Claims, error) {
	header, parts, err := decodeJWTHeader(tokenString)
	if err != nil {
		return token.Claims{}, err
	}
	// Strictly require RS256 (algorithm-confusion defense: never honor
	// alg:none or HMAC downgrade attempts).
	if header.Alg != "RS256" {
		return token.Claims{}, fmt.Errorf("jwt: verify: alg %q: %w", header.Alg, token.ErrUnexpectedAlg)
	}
	pub, ok := m.keys[header.Kid]
	if !ok {
		return token.Claims{}, fmt.Errorf("jwt: verify: kid %q not found: %w", header.Kid, token.ErrInvalidToken)
	}
	return verifyRS256(parts, pub)
}

// MultiKeyProvider implements token.KeyProvider for a set of public
// keys. JWKS publishes the active key first, followed by retired keys.
type MultiKeyProvider struct {
	activeKid string
	keys      map[string]*rsa.PublicKey // kid → public key
}

var _ token.KeyProvider = (*MultiKeyProvider)(nil)

// NewMultiKeyProvider builds a MultiKeyProvider.
func NewMultiKeyProvider(activeKid string, keys map[string]*rsa.PublicKey) *MultiKeyProvider {
	return &MultiKeyProvider{activeKid: activeKid, keys: keys}
}

// PublicJWK returns the active signing key's JWK.
func (p *MultiKeyProvider) PublicJWK() token.JWK {
	return rsaPublicKeyToJWK(p.keys[p.activeKid], p.activeKid)
}

// JWKS returns all public keys (active + retired) as a JWK Set. The
// active key is always listed first so verifiers find it quickly.
func (p *MultiKeyProvider) JWKS() token.JWKSet {
	jwks := make([]token.JWK, 0, len(p.keys))
	if pub, ok := p.keys[p.activeKid]; ok {
		jwks = append(jwks, rsaPublicKeyToJWK(pub, p.activeKid))
	}
	for kid, pub := range p.keys {
		if kid == p.activeKid {
			continue
		}
		jwks = append(jwks, rsaPublicKeyToJWK(pub, kid))
	}
	return token.JWKSet{Keys: jwks}
}

// rsaPublicKeyToJWK encodes an RSA public key as a token.JWK (RFC 7517
// / RFC 7518 6.3.1): kty=RSA, use=sig, alg=RS256, plus base64url n/e.
func rsaPublicKeyToJWK(pub *rsa.PublicKey, kid string) token.JWK {
	return token.JWK{
		Kty: "RSA",
		Use: "sig",
		Alg: "RS256",
		Kid: kid,
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}
}

// parseRSAPrivateKey decodes a PKCS#1 PEM-encoded RSA private key.
func parseRSAPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// parseRSAPublicKey decodes a PKIX PEM-encoded RSA public key
// (-----BEGIN PUBLIC KEY-----).
func parseRSAPublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}
	return rsaPub, nil
}
