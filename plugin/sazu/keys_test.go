package sazu

import "testing"

func TestKeyRegistryPinKSKAndGet(t *testing.T) {
	r := NewKeyRegistry()
	if _, ok := r.Get("example.org."); ok {
		t.Fatalf("expected no key pinned yet")
	}

	key, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	r.PinKSK("example.org.", key)

	got, ok := r.Get("example.org.")
	if !ok || got.KSK.DNSKEY.PublicKey != key.PublicKey {
		t.Fatalf("expected to get back the pinned KSK")
	}
	if len(got.ZSKs) != 0 {
		t.Fatalf("expected no ZSKs yet, got %d", len(got.ZSKs))
	}
}

func TestKeyRegistryNormalizesZoneNames(t *testing.T) {
	r := NewKeyRegistry()
	key, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	r.PinKSK("EXAMPLE.ORG", key) // no trailing dot, mixed case

	if _, ok := r.Get("example.org."); !ok {
		t.Fatalf("expected zone name lookup to be case- and FQDN-insensitive")
	}
}

func TestKeyRegistryAddZSKRequiresExistingKSK(t *testing.T) {
	r := NewKeyRegistry()
	zsk, _, err := GenerateEd25519Key("example.org.", false)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	if err := r.AddZSK("example.org.", zsk, true); err == nil {
		t.Fatalf("expected an error registering a ZSK with no KSK pinned yet")
	}
}

func TestKeyRegistryAddZSKThenGet(t *testing.T) {
	r := NewKeyRegistry()
	ksk, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating KSK: %v", err)
	}
	r.PinKSK("example.org.", ksk)

	zsk, _, err := GenerateEd25519Key("example.org.", false)
	if err != nil {
		t.Fatalf("generating ZSK: %v", err)
	}
	if err := r.AddZSK("example.org.", zsk, true); err != nil {
		t.Fatalf("AddZSK: %v", err)
	}

	got, ok := r.Get("example.org.")
	if !ok {
		t.Fatalf("expected zone keys to exist")
	}
	if len(got.ZSKs) != 1 || got.ZSKs[0].DNSKEY.PublicKey != zsk.PublicKey {
		t.Fatalf("expected the registered ZSK to come back, got %+v", got.ZSKs)
	}
	if !got.ZSKs[0].CanAuthenticateTx {
		t.Fatalf("expected the registered ZSK to be able to authenticate transactions")
	}
}

func TestKeyRegistryAddZSKRejectsDuplicateKeyTag(t *testing.T) {
	r := NewKeyRegistry()
	ksk, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating KSK: %v", err)
	}
	r.PinKSK("example.org.", ksk)

	zsk, _, err := GenerateEd25519Key("example.org.", false)
	if err != nil {
		t.Fatalf("generating ZSK: %v", err)
	}
	if err := r.AddZSK("example.org.", zsk, true); err != nil {
		t.Fatalf("first AddZSK: %v", err)
	}
	if err := r.AddZSK("example.org.", zsk, true); err == nil {
		t.Fatalf("expected an error registering the same key tag twice")
	}
}

func TestKeyRegistryRetireZSK(t *testing.T) {
	r := NewKeyRegistry()
	ksk, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating KSK: %v", err)
	}
	r.PinKSK("example.org.", ksk)

	zsk, _, err := GenerateEd25519Key("example.org.", false)
	if err != nil {
		t.Fatalf("generating ZSK: %v", err)
	}
	if err := r.AddZSK("example.org.", zsk, true); err != nil {
		t.Fatalf("AddZSK: %v", err)
	}

	if removed := r.RetireZSK("example.org.", zsk.KeyTag()); !removed {
		t.Fatalf("expected RetireZSK to report the ZSK was found and removed")
	}
	got, _ := r.Get("example.org.")
	if len(got.ZSKs) != 0 {
		t.Fatalf("expected no ZSKs left after retiring the only one, got %d", len(got.ZSKs))
	}
}

func TestKeyRegistryRetireZSKOfUnknownKeytagIsANoop(t *testing.T) {
	r := NewKeyRegistry()
	ksk, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating KSK: %v", err)
	}
	r.PinKSK("example.org.", ksk)

	if removed := r.RetireZSK("example.org.", 12345); removed {
		t.Fatalf("expected retiring an unknown key tag to report nothing removed")
	}
}

func TestKeyRegistryPinKSKPreservesExistingZSKs(t *testing.T) {
	r := NewKeyRegistry()
	ksk1, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating first KSK: %v", err)
	}
	r.PinKSK("example.org.", ksk1)

	zsk, _, err := GenerateEd25519Key("example.org.", false)
	if err != nil {
		t.Fatalf("generating ZSK: %v", err)
	}
	if err := r.AddZSK("example.org.", zsk, true); err != nil {
		t.Fatalf("AddZSK: %v", err)
	}

	ksk2, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating second KSK: %v", err)
	}
	r.PinKSK("example.org.", ksk2) // simulates a KSK rollover

	got, _ := r.Get("example.org.")
	if got.KSK.DNSKEY.PublicKey != ksk2.PublicKey {
		t.Fatalf("expected the KSK to have rolled over to the new one")
	}
	if len(got.ZSKs) != 1 || got.ZSKs[0].DNSKEY.PublicKey != zsk.PublicKey {
		t.Fatalf("expected the ZSK to survive a KSK rollover untouched, got %+v", got.ZSKs)
	}
}

func TestZoneKeysAuthenticatorsIncludesOnlyCanAuthenticateTxZSKs(t *testing.T) {
	r := NewKeyRegistry()
	ksk, _, err := GenerateEd25519Key("example.org.", true)
	if err != nil {
		t.Fatalf("generating KSK: %v", err)
	}
	r.PinKSK("example.org.", ksk)

	authZSK, _, err := GenerateEd25519Key("example.org.", false)
	if err != nil {
		t.Fatalf("generating ZSK: %v", err)
	}
	if err := r.AddZSK("example.org.", authZSK, true); err != nil {
		t.Fatalf("AddZSK: %v", err)
	}

	zk, _ := r.Get("example.org.")
	auths := zk.Authenticators()
	if len(auths) != 2 {
		t.Fatalf("expected KSK + the authenticating ZSK, got %d authenticators", len(auths))
	}

	signers := zk.ContentSigners()
	if len(signers) != 2 {
		t.Fatalf("expected KSK + ZSK as content signers, got %d", len(signers))
	}
}
