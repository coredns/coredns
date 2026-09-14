package sazu

import (
	"strings"
	"testing"
)

func TestEncryptDecryptKeyFileBytesRoundTrip(t *testing.T) {
	plaintext := []byte("Private-key-format: v1.3\nAlgorithm: 15 (ED25519)\nPrivateKey: c2VjcmV0\n")
	enc, err := encryptKeyFileBytes(plaintext, []byte("hunter2"))
	if err != nil {
		t.Fatalf("encryptKeyFileBytes: %v", err)
	}
	if !isEncryptedKeyFile(enc) {
		t.Fatalf("expected encrypted output to be recognized as an encrypted key file")
	}
	got, err := decryptKeyFileBytes(enc, []byte("hunter2"))
	if err != nil {
		t.Fatalf("decryptKeyFileBytes: %v", err)
	}
	if string(got) != string(plaintext) {
		t.Fatalf("decrypted plaintext differs: got %q, want %q", got, plaintext)
	}
}

func TestDecryptKeyFileBytesRejectsWrongPassphrase(t *testing.T) {
	enc, err := encryptKeyFileBytes([]byte("secret"), []byte("right"))
	if err != nil {
		t.Fatalf("encryptKeyFileBytes: %v", err)
	}
	if _, err := decryptKeyFileBytes(enc, []byte("wrong")); err == nil {
		t.Fatalf("expected the wrong passphrase to be rejected")
	}
}

func TestDecryptKeyFileBytesRejectsTamperedCiphertext(t *testing.T) {
	enc, err := encryptKeyFileBytes([]byte("secret"), []byte("passphrase"))
	if err != nil {
		t.Fatalf("encryptKeyFileBytes: %v", err)
	}
	// Flip a byte inside the JSON envelope's ciphertext field -- AES-GCM's
	// authentication tag must catch this, not just silently decrypt to
	// garbage.
	tampered := strings.Replace(string(enc), "a", "b", 1)
	if _, err := decryptKeyFileBytes([]byte(tampered), []byte("passphrase")); err == nil {
		t.Fatalf("expected a tampered ciphertext to be rejected")
	}
}

func TestIsEncryptedKeyFileDistinguishesFromPlainBINDFormat(t *testing.T) {
	plainBIND := []byte("Private-key-format: v1.3\nAlgorithm: 15 (ED25519)\nPrivateKey: c2VjcmV0\n")
	if isEncryptedKeyFile(plainBIND) {
		t.Fatalf("expected a plain BIND-format key file to not be recognized as encrypted")
	}
}

func TestDecryptKeyFileBytesRejectsNonEncryptedInput(t *testing.T) {
	plainBIND := []byte("Private-key-format: v1.3\nPrivateKey: c2VjcmV0\n")
	if _, err := decryptKeyFileBytes(plainBIND, []byte("whatever")); err == nil {
		t.Fatalf("expected decrypting a plain, non-encrypted file to fail")
	}
}
