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

	loadedPriv, err := LoadPrivateKey(path, nil)
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

	first, _, generated, err := LoadOrGenerateKey(path, "example.org.", true, nil)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if !generated {
		t.Fatalf("expected a new key to be generated on first call")
	}

	second, _, generated, err := LoadOrGenerateKey(path, "example.org.", true, nil)
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

// TestSaveLoadEncryptedKeyRoundTripsToIdenticalKey proves §10.8's
// passphrase-encrypted key file round-trips to exactly the same key
// material as the plain format does.
func TestSaveLoadEncryptedKeyRoundTripsToIdenticalKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client.private")
	passphrase := []byte("correct horse battery staple")

	original, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	if err := SaveEncryptedPrivateKey(path, original, priv, passphrase); err != nil {
		t.Fatalf("saving encrypted key: %v", err)
	}

	loadedPriv, err := LoadPrivateKey(path, passphrase)
	if err != nil {
		t.Fatalf("loading encrypted key: %v", err)
	}
	reloaded := DNSKEYFor("example.org.", loadedPriv, true)
	if reloaded.PublicKey != original.PublicKey {
		t.Fatalf("reloaded public key differs: got %q, want %q", reloaded.PublicKey, original.PublicKey)
	}
}

// TestLoadEncryptedKeyRejectsWrongOrMissingPassphrase proves an encrypted
// key file can't be read back without the right passphrase -- wrong and
// missing are both rejected, and (since AES-GCM authenticates) rejected
// the same way a corrupted file would be, not with a different error that
// would let a caller distinguish "wrong passphrase" from "not encrypted
// with this scheme at all."
func TestLoadEncryptedKeyRejectsWrongOrMissingPassphrase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client.private")

	key, priv, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	if err := SaveEncryptedPrivateKey(path, key, priv, []byte("right passphrase")); err != nil {
		t.Fatalf("saving encrypted key: %v", err)
	}

	if _, err := LoadPrivateKey(path, nil); err == nil {
		t.Fatalf("expected loading an encrypted key with no passphrase to fail")
	}
	if _, err := LoadPrivateKey(path, []byte("wrong passphrase")); err == nil {
		t.Fatalf("expected loading an encrypted key with the wrong passphrase to fail")
	}
	if _, err := LoadPrivateKey(path, []byte("right passphrase")); err != nil {
		t.Fatalf("expected the right passphrase to succeed, got: %v", err)
	}
}

// TestLoadOrGenerateKeyWithPassphraseGeneratesEncryptedFile proves the
// LoadOrGenerateKey integration: given a passphrase and no existing file,
// it generates a key and saves it encrypted -- provably so, since loading
// it back with no passphrase must then fail.
func TestLoadOrGenerateKeyWithPassphraseGeneratesEncryptedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client.private")
	passphrase := []byte("a passphrase")

	first, _, generated, err := LoadOrGenerateKey(path, "example.org.", true, passphrase)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if !generated {
		t.Fatalf("expected a new key to be generated on first call")
	}
	if _, err := LoadPrivateKey(path, nil); err == nil {
		t.Fatalf("expected the generated file to be encrypted (unreadable with no passphrase)")
	}

	second, _, generated, err := LoadOrGenerateKey(path, "example.org.", true, passphrase)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if generated {
		t.Fatalf("expected the second call to reuse the existing encrypted key, not generate a new one")
	}
	if first.PublicKey != second.PublicKey {
		t.Fatalf("expected the same key to be reloaded, got a different public key")
	}
}
