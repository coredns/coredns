package sazu

import (
	"path/filepath"
	"testing"

	"github.com/miekg/dns"
)

func TestSaveLoadKeyRoundTripsToIdenticalDNSKEYAndDS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client.private")

	original, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	if err := SavePrivateKey(path, original, priv); err != nil {
		t.Fatalf("saving key: %v", err)
	}

	loadedPriv, err := LoadPrivateKey(path)
	if err != nil {
		t.Fatalf("loading key: %v", err)
	}
	reloaded := DNSKEYFor("example.org.", loadedPriv, true)

	if reloaded.PublicKey != original.PublicKey {
		t.Fatalf("reloaded public key differs: got %q, want %q", reloaded.PublicKey, original.PublicKey)
	}
	if reloaded.KeyTag() != original.KeyTag() {
		t.Fatalf("reloaded key tag differs: got %d, want %d", reloaded.KeyTag(), original.KeyTag())
	}

	originalDS := original.ToDS(dns.SHA256)
	reloadedDS := reloaded.ToDS(dns.SHA256)
	if originalDS.Digest != reloadedDS.Digest {
		t.Fatalf("reloaded DS digest differs: got %s, want %s", reloadedDS.Digest, originalDS.Digest)
	}
}

func TestLoadOrGenerateKeyGeneratesThenReusesSameKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client.private")

	first, _, generated, err := LoadOrGenerateKey(path, "example.org.", true)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if !generated {
		t.Fatalf("expected a new key to be generated on first call")
	}

	second, _, generated, err := LoadOrGenerateKey(path, "example.org.", true)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if generated {
		t.Fatalf("expected the second call to reuse the existing key, not generate a new one")
	}
	if first.PublicKey != second.PublicKey {
		t.Fatalf("expected the same key to be reloaded, got a different public key")
	}
}
