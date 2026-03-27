package hanzodns

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type contextKey string

const orgIDKey contextKey = "org_id"

// OrgIDFromContext returns the org_id extracted from the JWT token.
func OrgIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(orgIDKey).(string)
	return v
}

// oidcMiddleware validates OIDC JWT tokens when HANZO_DNS_OIDC_ISSUER is set.
func oidcMiddleware(next http.Handler) http.Handler {
	issuer := os.Getenv("HANZO_DNS_OIDC_ISSUER")
	if issuer == "" {
		return next
	}

	audience := os.Getenv("HANZO_DNS_OIDC_AUDIENCE")
	cache := &jwksCache{issuer: issuer}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing Authorization header")
			return
		}

		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid Authorization header")
			return
		}

		keys, err := cache.getKeys()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to fetch JWKS")
			return
		}

		claims, err := validateToken(parts[1], keys, issuer, audience)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", err.Error())
			return
		}

		ctx := context.WithValue(r.Context(), orgIDKey, claims.OrgID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type jwtClaims struct {
	Issuer   string `json:"iss"`
	Subject  string `json:"sub"`
	Audience string `json:"aud"`
	Expiry   int64  `json:"exp"`
	IssuedAt int64  `json:"iat"`
	OrgID    string `json:"org_id"`
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func (k jwk) toPublicKey() (crypto.PublicKey, error) {
	switch k.Kty {
	case "RSA":
		nb, err := base64URLDecode(k.N)
		if err != nil {
			return nil, err
		}
		eb, err := base64URLDecode(k.E)
		if err != nil {
			return nil, err
		}
		return &rsa.PublicKey{
			N: new(big.Int).SetBytes(nb),
			E: int(new(big.Int).SetBytes(eb).Int64()),
		}, nil
	case "EC":
		xb, err := base64URLDecode(k.X)
		if err != nil {
			return nil, err
		}
		yb, err := base64URLDecode(k.Y)
		if err != nil {
			return nil, err
		}
		var curve elliptic.Curve
		switch k.Crv {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		default:
			return nil, fmt.Errorf("unsupported curve: %s", k.Crv)
		}
		return &ecdsa.PublicKey{Curve: curve, X: new(big.Int).SetBytes(xb), Y: new(big.Int).SetBytes(yb)}, nil
	default:
		return nil, fmt.Errorf("unsupported key type: %s", k.Kty)
	}
}

type jwksCache struct {
	issuer    string
	mu        sync.RWMutex
	keys      []jwk
	fetchedAt time.Time
}

func (c *jwksCache) getKeys() ([]jwk, error) {
	c.mu.RLock()
	if time.Since(c.fetchedAt) < time.Hour && len(c.keys) > 0 {
		keys := c.keys
		c.mu.RUnlock()
		return keys, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Since(c.fetchedAt) < time.Hour && len(c.keys) > 0 {
		return c.keys, nil
	}

	keys, err := fetchJWKS(c.issuer)
	if err != nil {
		if len(c.keys) > 0 {
			return c.keys, nil
		}
		return nil, err
	}
	c.keys = keys
	c.fetchedAt = time.Now()
	return keys, nil
}

func fetchJWKS(issuerURL string) ([]jwk, error) {
	issuerURL = strings.TrimSuffix(issuerURL, "/")
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(issuerURL + "/.well-known/openid-configuration")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var config struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, err
	}
	if config.JWKSURI == "" {
		return nil, fmt.Errorf("no jwks_uri in oidc config")
	}

	resp2, err := client.Get(config.JWKSURI)
	if err != nil {
		return nil, err
	}
	defer resp2.Body.Close()

	var jwks struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&jwks); err != nil {
		return nil, err
	}
	return jwks.Keys, nil
}

func validateToken(tokenStr string, keys []jwk, issuer, audience string) (*jwtClaims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	headerBytes, err := base64URLDecode(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, err
	}

	var matchedKey *jwk
	for _, k := range keys {
		if k.Kid == header.Kid || (header.Kid == "" && k.Use == "sig") {
			matchedKey = &k
			break
		}
	}
	if matchedKey == nil {
		return nil, fmt.Errorf("no matching key for kid %q", header.Kid)
	}

	signingInput := parts[0] + "." + parts[1]
	signature, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}

	pubKey, err := matchedKey.toPublicKey()
	if err != nil {
		return nil, err
	}

	hash := sha256.Sum256([]byte(signingInput))
	switch key := pubKey.(type) {
	case *rsa.PublicKey:
		if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], signature); err != nil {
			return nil, fmt.Errorf("invalid signature")
		}
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(key, hash[:], signature) {
			return nil, fmt.Errorf("invalid signature")
		}
	default:
		return nil, fmt.Errorf("unsupported key type")
	}

	claimsBytes, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, err
	}
	var c jwtClaims
	if err := json.Unmarshal(claimsBytes, &c); err != nil {
		return nil, err
	}

	if c.Expiry > 0 && time.Now().Unix() > c.Expiry {
		return nil, fmt.Errorf("token expired")
	}
	if c.Issuer != issuer {
		return nil, fmt.Errorf("invalid issuer: got %q, want %q", c.Issuer, issuer)
	}
	if audience != "" && c.Audience != audience {
		return nil, fmt.Errorf("invalid audience")
	}

	return &c, nil
}

func base64URLDecode(s string) ([]byte, error) {
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}
