package sazu

import "testing"

func TestKeyRegistryPinAndGet(t *testing.T) {
	r := NewKeyRegistry()
	if _, ok := r.Get("example.org."); ok {
		t.Fatalf("expected no key pinned yet")
	}

	key, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	r.Pin("example.org.", key)

	got, ok := r.Get("example.org.")
	if !ok || got.PublicKey != key.PublicKey {
		t.Fatalf("expected to get back the pinned key")
	}
}

func TestKeyRegistryNormalizesZoneNames(t *testing.T) {
	r := NewKeyRegistry()
	key, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	r.Pin("EXAMPLE.ORG", key) // no trailing dot, mixed case

	if _, ok := r.Get("example.org."); !ok {
		t.Fatalf("expected zone name lookup to be case- and FQDN-insensitive")
	}
}
