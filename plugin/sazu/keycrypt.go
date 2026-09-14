package sazu

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"golang.org/x/crypto/scrypt"
)

// §10.8 key custody hardening: sazuctl's private-key file is, by default,
// BIND9's plain private-key-file format (see key.go) -- readable by
// standard DNSSEC tooling, but also readable by anyone who gets the file.
// This adds an opt-in, passphrase-encrypted alternative: still a single
// self-contained file, still no external key-management service or
// hardware dependency, but no longer plaintext at rest. Real HSM/PKCS#11
// support (holding the key in hardware entirely, never as bytes on disk
// at all) is a materially bigger step -- a new dependency, a real or
// software HSM to test against, and an API redesign for signing -- and is
// left for if/when this actually needs that; this covers the common case
// (a laptop or CI secret store gets compromised or synced somewhere it
// shouldn't) without it.
//
// Encrypted file format: a fixed magic line (so LoadPrivateKey can tell
// this apart from a plain BIND-format file at a glance, without first
// trying and failing to parse it as one) followed by a JSON envelope
// carrying everything needed to decrypt except the passphrase itself:
// the scrypt parameters and salt used to derive the AES-256 key, the
// AES-GCM nonce, and the ciphertext. The plaintext sealed inside is
// exactly what SavePrivateKey would have written -- the ordinary
// BIND-format key file bytes -- so decrypting one out-of-band (should
// that ever be needed) yields a file usable with standard tooling too.
const encryptedKeyFileMagic = "SAZU-ENCRYPTED-KEY-v1\n"

// scrypt parameters: N=2^15 costs roughly 100-200ms on typical hardware
// for this CLI tool's own use (a human running a command, not a hot
// path) -- deliberately above scrypt's original "interactive login"
// recommendation of 2^14, since this protects a long-lived signing key,
// not a session login.
const (
	scryptN      = 1 << 15
	scryptR      = 8
	scryptP      = 1
	scryptKeyLen = 32 // AES-256
	saltLen      = 16
)

type encryptedKeyEnvelope struct {
	Salt       string `json:"salt"`
	N          int    `json:"n"`
	R          int    `json:"r"`
	P          int    `json:"p"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

// isEncryptedKeyFile reports whether data is this package's own
// passphrase-encrypted key file format, as opposed to a plain BIND
// private-key-file.
func isEncryptedKeyFile(data []byte) bool {
	return bytes.HasPrefix(data, []byte(encryptedKeyFileMagic))
}

// encryptKeyFileBytes encrypts plaintext (a BIND-format key file's own
// bytes, as SavePrivateKey would write) with a key derived from
// passphrase via scrypt, returning the complete on-disk representation
// (magic line plus JSON envelope).
func encryptKeyFileBytes(plaintext, passphrase []byte) ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generating salt: %w", err)
	}
	key, err := scrypt.Key(passphrase, salt, scryptN, scryptR, scryptP, scryptKeyLen)
	if err != nil {
		return nil, fmt.Errorf("deriving key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	env := encryptedKeyEnvelope{
		Salt:       base64.StdEncoding.EncodeToString(salt),
		N:          scryptN,
		R:          scryptR,
		P:          scryptP,
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}
	body, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	return append([]byte(encryptedKeyFileMagic), body...), nil
}

// decryptKeyFileBytes reverses encryptKeyFileBytes, given the exact bytes
// read from disk (magic line included) and the passphrase. A wrong
// passphrase is indistinguishable from a corrupted file here -- AES-GCM
// authentication fails either way -- which is the correct, safe ambiguity
// to present (never confirm or deny which one it was via a different
// error path).
func decryptKeyFileBytes(data, passphrase []byte) ([]byte, error) {
	body, ok := bytes.CutPrefix(data, []byte(encryptedKeyFileMagic))
	if !ok {
		return nil, fmt.Errorf("not a SAZU-encrypted key file")
	}
	var env encryptedKeyEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parsing encrypted key envelope: %w", err)
	}
	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil {
		return nil, fmt.Errorf("decoding salt: %w", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decoding nonce: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decoding ciphertext: %w", err)
	}

	key, err := scrypt.Key(passphrase, salt, env.N, env.R, env.P, scryptKeyLen)
	if err != nil {
		return nil, fmt.Errorf("deriving key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypting key file (wrong passphrase?): %w", err)
	}
	return plaintext, nil
}
