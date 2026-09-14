package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestGuidanceTemplatesRenderWithoutError proves every embedded guidance
// template (guidance/*.txt) actually executes against the data type its
// caller passes it -- template.Must at package init only catches a
// syntax error, not a template referencing a field its data struct
// doesn't have, which only fails at Execute time. Also a basic sanity
// check that no template silently renders empty.
func TestGuidanceTemplatesRenderWithoutError(t *testing.T) {
	dsData := dsGuidanceData{
		Zone: "example.org.", Owner: "example.org.",
		KeyTag: 12345, Algorithm: 15, AlgorithmName: "ED25519",
		DigestType: 2, Digest: "abcd1234",
		KeyTypeValue: 257, KeyTypeLabel: "KSK",
		PublicKeyB64: "AAAA",
	}
	rotateData := rotateKeyChoiceData{Zone: "example.org.", HasZSK: true, ZSKKeyTag: 54321}

	cases := []struct {
		name string
		data any
	}{
		{"no-ds.txt", dsData},
		{"unknown-signer.txt", dsData},
		{"rotate-key-choice.txt", rotateData},
		{"rotate-key-choice.txt", rotateKeyChoiceData{Zone: "example.org.", HasZSK: false}},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if err := guidanceTemplates.ExecuteTemplate(&buf, c.name, c.data); err != nil {
			t.Fatalf("executing %s: %v", c.name, err)
		}
		if buf.Len() == 0 {
			t.Fatalf("expected %s to render non-empty output", c.name)
		}
	}
}

// TestNoDSGuidanceIncludesTheDSRecordAndZone proves the rendered output
// actually contains the concrete, actionable values a customer needs --
// not just that the template executes without error.
func TestNoDSGuidanceIncludesTheDSRecordAndZone(t *testing.T) {
	var buf bytes.Buffer
	data := dsGuidanceData{
		Zone: "example.org.", Owner: "example.org.",
		KeyTag: 12345, Algorithm: 15, AlgorithmName: "ED25519",
		DigestType: 2, Digest: "deadbeef",
		KeyTypeValue: 257, KeyTypeLabel: "KSK",
		PublicKeyB64: "base64stuff",
	}
	if err := guidanceTemplates.ExecuteTemplate(&buf, "no-ds.txt", data); err != nil {
		t.Fatalf("executing no-ds.txt: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"example.org.", "12345", "deadbeef", "base64stuff", "dig DS example.org. +short"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected rendered no-ds.txt to contain %q, got:\n%s", want, out)
		}
	}
}

// TestRotateKeyChoiceGuidanceReflectsWhetherAZSKExists proves the
// decision text actually branches on HasZSK, rather than always
// printing the same generic advice regardless of the zone's real state.
func TestRotateKeyChoiceGuidanceReflectsWhetherAZSKExists(t *testing.T) {
	var withZSK, withoutZSK bytes.Buffer
	if err := guidanceTemplates.ExecuteTemplate(&withZSK, "rotate-key-choice.txt",
		rotateKeyChoiceData{Zone: "example.org.", HasZSK: true, ZSKKeyTag: 999}); err != nil {
		t.Fatalf("executing with HasZSK=true: %v", err)
	}
	if err := guidanceTemplates.ExecuteTemplate(&withoutZSK, "rotate-key-choice.txt",
		rotateKeyChoiceData{Zone: "example.org.", HasZSK: false}); err != nil {
		t.Fatalf("executing with HasZSK=false: %v", err)
	}
	if !strings.Contains(withZSK.String(), "999") {
		t.Fatalf("expected the existing ZSK's key tag to appear when HasZSK is true, got:\n%s", withZSK.String())
	}
	if strings.Contains(withoutZSK.String(), "add-zsk -zone example.org. -ksk-key") == false {
		t.Fatalf("expected guidance to register a ZSK first when none exists yet, got:\n%s", withoutZSK.String())
	}
	if withZSK.String() == withoutZSK.String() {
		t.Fatalf("expected the two HasZSK states to render different guidance")
	}
}

// TestKeyTypeLabelAndAlgorithmLabel proves the small display helpers the
// guidance templates and printKeyInfo depend on are correct.
func TestKeyTypeLabelAndAlgorithmLabel(t *testing.T) {
	if got := keyTypeLabel(257); got != "KSK" {
		t.Fatalf("keyTypeLabel(257) = %q, want KSK", got)
	}
	if got := keyTypeLabel(256); got != "ZSK" {
		t.Fatalf("keyTypeLabel(256) = %q, want ZSK", got)
	}
	if got := keyTypeLabel(1); got != "unrecognized flags" {
		t.Fatalf("keyTypeLabel(1) = %q, want \"unrecognized flags\"", got)
	}
	if got := algorithmLabel(15); got != "ED25519" {
		t.Fatalf("algorithmLabel(15) = %q, want ED25519", got)
	}
}

// TestParseRoleFlag proves the -role flag's three cases -- accepted,
// accepted, and rejected -- since add-zsk/retire-zsk/rotate-key/keygen
// all depend on it to decide whether a key is a KSK or a ZSK.
func TestParseRoleFlag(t *testing.T) {
	if ksk, err := parseRoleFlag("ksk"); err != nil || !ksk {
		t.Fatalf(`parseRoleFlag("ksk") = %v, %v; want true, nil`, ksk, err)
	}
	if ksk, err := parseRoleFlag("zsk"); err != nil || ksk {
		t.Fatalf(`parseRoleFlag("zsk") = %v, %v; want false, nil`, ksk, err)
	}
	if _, err := parseRoleFlag("neither"); err == nil {
		t.Fatalf(`parseRoleFlag("neither") = nil error, want an error`)
	}
}
