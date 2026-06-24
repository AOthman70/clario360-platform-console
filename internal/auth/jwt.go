package auth

import (
	"crypto/rsa"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// Claims is the platform JWT payload. The gateway issues RS256-signed tokens
// (Slide 5: "RS256 JWT"). Beyond the registered claims we carry the caller's
// tenant, role slugs, and the flattened permission set the gate evaluates.
//
// The impersonation fields (Slide 9) are populated only inside an approved
// act-as session: ActAsTenant identifies the impersonated tenant, ReadOnly
// reflects the minted token's readonly claim, and ActAsBy attributes the
// session back to the originating operator.
type Claims struct {
	jwt.RegisteredClaims

	TenantID    string   `json:"tenant_id"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`

	// Impersonation (act-as) context — empty outside an impersonation session.
	ActAsTenant string `json:"act_as_tenant,omitempty"`
	ReadOnly    bool   `json:"readonly,omitempty"`
	ActAsBy     string `json:"act_as_by,omitempty"`
}

// IsImpersonating reports whether the request is running inside an act-as
// session. Destructive handlers must refuse when this is true and ReadOnly is
// set.
func (c *Claims) IsImpersonating() bool { return c.ActAsTenant != "" }

// Verifier validates RS256 tokens against a public key. Construct one per
// signing key via NewVerifier and share it across requests (it is read-only
// and safe for concurrent use).
type Verifier struct {
	pub      *rsa.PublicKey
	issuer   string
	audience string
}

// NewVerifier builds a Verifier from a PEM-encoded RSA public key. issuer and
// audience, when non-empty, are enforced on every token.
func NewVerifier(pemKey []byte, issuer, audience string) (*Verifier, error) {
	pub, err := jwt.ParseRSAPublicKeyFromPEM(pemKey)
	if err != nil {
		return nil, fmt.Errorf("auth: parse RSA public key: %w", err)
	}
	return &Verifier{pub: pub, issuer: issuer, audience: audience}, nil
}

// Parse validates the token string and returns its claims. It enforces the
// RS256 algorithm explicitly — never trusting the token's own alg header — to
// close the alg-confusion class of attack.
func (v *Verifier) Parse(token string) (*Claims, error) {
	opts := []jwt.ParserOption{jwt.WithValidMethods([]string{"RS256"})}
	if v.issuer != "" {
		opts = append(opts, jwt.WithIssuer(v.issuer))
	}
	if v.audience != "" {
		opts = append(opts, jwt.WithAudience(v.audience))
	}

	claims := &Claims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return v.pub, nil
	}, opts...)
	if err != nil {
		return nil, fmt.Errorf("auth: invalid token: %w", err)
	}
	return claims, nil
}
