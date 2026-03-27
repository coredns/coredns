package hanzodns

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func generateTestKey() (*rsa.PrivateKey, jwk) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	pub := key.PublicKey

	jwkKey := jwk{
		Kty: "RSA",
		Kid: "test-key-1",
		Alg: "RS256",
		Use: "sig",
		N:   base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(pub.N.Bytes()),
		E:   base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}

	return key, jwkKey
}

func signJWT(key *rsa.PrivateKey, header, payload map[string]interface{}) string {
	hb, _ := json.Marshal(header)
	pb, _ := json.Marshal(payload)

	h := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hb)
	p := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(pb)

	signingInput := h + "." + p
	hash := sha256.Sum256([]byte(signingInput))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])

	s := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(sig)
	return h + "." + p + "." + s
}

func mockJWKSServer(jwkKey jwk) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]string{
				"issuer":   fmt.Sprintf("http://%s", r.Host),
				"jwks_uri": fmt.Sprintf("http://%s/.well-known/jwks", r.Host),
			})
		case "/.well-known/jwks":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"keys": []jwk{jwkKey},
			})
		default:
			w.WriteHeader(404)
		}
	}))
}

func TestValidToken(t *testing.T) {
	key, jwkKey := generateTestKey()
	srv := mockJWKSServer(jwkKey)
	defer srv.Close()

	token := signJWT(key, map[string]interface{}{
		"alg": "RS256",
		"kid": "test-key-1",
	}, map[string]interface{}{
		"iss":    srv.URL,
		"sub":    "user-123",
		"org_id": "org-456",
		"exp":    time.Now().Add(time.Hour).Unix(),
	})

	claims, err := validateToken(token, []jwk{jwkKey}, srv.URL, "")
	if err != nil {
		t.Fatalf("expected valid token, got: %v", err)
	}
	if claims.OrgID != "org-456" {
		t.Fatalf("expected org_id org-456, got %s", claims.OrgID)
	}
	if claims.Subject != "user-123" {
		t.Fatalf("expected sub user-123, got %s", claims.Subject)
	}
}

func TestExpiredToken(t *testing.T) {
	key, jwkKey := generateTestKey()

	token := signJWT(key, map[string]interface{}{
		"alg": "RS256",
		"kid": "test-key-1",
	}, map[string]interface{}{
		"iss": "https://hanzo.id",
		"exp": time.Now().Add(-time.Hour).Unix(),
	})

	_, err := validateToken(token, []jwk{jwkKey}, "https://hanzo.id", "")
	if err == nil || err.Error() != "token expired" {
		t.Fatalf("expected 'token expired', got: %v", err)
	}
}

func TestWrongIssuer(t *testing.T) {
	key, jwkKey := generateTestKey()

	token := signJWT(key, map[string]interface{}{
		"alg": "RS256",
		"kid": "test-key-1",
	}, map[string]interface{}{
		"iss": "https://evil.com",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	_, err := validateToken(token, []jwk{jwkKey}, "https://hanzo.id", "")
	if err == nil {
		t.Fatal("expected error for wrong issuer")
	}
}

func TestMissingToken(t *testing.T) {
	_, err := validateToken("", nil, "https://hanzo.id", "")
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestMiddlewareDisabledWhenEnvUnset(t *testing.T) {
	os.Unsetenv("HANZO_DNS_OIDC_ISSUER")

	called := false
	handler := oidcMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Fatal("expected handler to be called when OIDC disabled")
	}
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMiddlewareRejects401WithoutToken(t *testing.T) {
	key, jwkKey := generateTestKey()
	srv := mockJWKSServer(jwkKey)
	defer srv.Close()

	os.Setenv("HANZO_DNS_OIDC_ISSUER", srv.URL)
	defer os.Unsetenv("HANZO_DNS_OIDC_ISSUER")

	_ = key // ensure key is generated

	handler := oidcMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
