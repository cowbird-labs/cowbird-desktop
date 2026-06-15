package crypto

import (
	"bytes"
	"crypto/ed25519"
	"testing"
)

func TestNewIdentity(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}

	var zero [32]byte
	if id.EncryptionPub == zero {
		t.Fatal("public key must not be zero")
	}
	if id.EncryptionPriv == zero {
		t.Fatal("private key must not be zero")
	}
	if id.Fingerprint == "" {
		t.Fatal("fingerprint must not be empty")
	}

	if len(id.SigningPriv) != ed25519.PrivateKeySize {
		t.Fatalf("signing private key size = %d, want %d", len(id.SigningPriv), ed25519.PrivateKeySize)
	}
	if len(id.SigningPub) != ed25519.PublicKeySize {
		t.Fatalf("signing public key size = %d, want %d", len(id.SigningPub), ed25519.PublicKeySize)
	}

	id2, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if id.EncryptionPub == id2.EncryptionPub {
		t.Fatal("two identities must have different public keys")
	}
	if id.SigningPub.Equal(id2.SigningPub) {
		t.Fatal("two identities must have different signing keys")
	}
}

func TestLockUnlockPreservesSigningKey(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	locked, err := LockIdentity(id, []byte("pw"))
	if err != nil {
		t.Fatal(err)
	}
	unlocked, err := UnlockIdentity(locked, []byte("pw"))
	if err != nil {
		t.Fatal(err)
	}
	if !unlocked.SigningPriv.Equal(id.SigningPriv) {
		t.Fatal("signing private key mismatch after unlock")
	}
	if !unlocked.SigningPub.Equal(id.SigningPub) {
		t.Fatal("signing public key mismatch after unlock")
	}
}

func TestEnsureSigningKey(t *testing.T) {
	// A legacy identity with no signing key gets one; a second call is a no-op.
	id := &Identity{}
	added, err := id.EnsureSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Fatal("expected a signing key to be added to a legacy identity")
	}
	if len(id.SigningPriv) != ed25519.PrivateKeySize {
		t.Fatal("signing key not populated")
	}
	priv := id.SigningPriv
	added, err = id.EnsureSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	if added {
		t.Fatal("EnsureSigningKey must be a no-op when a key already exists")
	}
	if !priv.Equal(id.SigningPriv) {
		t.Fatal("EnsureSigningKey must not replace an existing key")
	}
}

func TestLockUnlockIdentity(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	password := []byte("test-password")

	locked, err := LockIdentity(id, password)
	if err != nil {
		t.Fatal(err)
	}
	if len(locked.Salt) == 0 || len(locked.Nonce) == 0 || len(locked.Ciphertext) == 0 {
		t.Fatal("locked identity must have non-empty salt, nonce, and ciphertext")
	}

	unlocked, err := UnlockIdentity(locked, password)
	if err != nil {
		t.Fatal(err)
	}
	if unlocked.EncryptionPriv != id.EncryptionPriv {
		t.Fatal("private key mismatch after unlock")
	}
	if unlocked.EncryptionPub != id.EncryptionPub {
		t.Fatal("public key mismatch after unlock")
	}
	if unlocked.Fingerprint != id.Fingerprint {
		t.Fatal("fingerprint mismatch after unlock")
	}
}

func TestUnlockIdentityWrongPassword(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	locked, err := LockIdentity(id, []byte("correct"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = UnlockIdentity(locked, []byte("wrong"))
	if err == nil {
		t.Fatal("expected error with wrong password")
	}
}

func TestLockProducesDistinctCiphertexts(t *testing.T) {
	id, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	password := []byte("same-password")

	l1, err := LockIdentity(id, password)
	if err != nil {
		t.Fatal(err)
	}
	l2, err := LockIdentity(id, password)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(l1.Salt, l2.Salt) {
		t.Fatal("two locks must have different salts")
	}
	if bytes.Equal(l1.Ciphertext, l2.Ciphertext) {
		t.Fatal("two locks must produce different ciphertexts")
	}
}