package crypto

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// Identity holds a user's keypair material in memory.
// Private key fields are populated only after unlock.
// Ed25519 fields are reserved for optional authorship signing (deferred).
type Identity struct {
	SigningPub     ed25519.PublicKey  // optional; deferred
	SigningPriv    ed25519.PrivateKey // optional; deferred
	EncryptionPub  [32]byte           // X25519 public key
	EncryptionPriv [32]byte           // X25519 private key
	Fingerprint    string             // hex-encoded SHA-256 of EncryptionPub
}

// NewIdentity generates a fresh X25519 encryption keypair (for wrapping item
// keys) and an Ed25519 signing keypair (for authenticating shares).
func NewIdentity() (*Identity, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating X25519 key: %w", err)
	}
	sigPub, sigPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating Ed25519 key: %w", err)
	}
	id := &Identity{SigningPub: sigPub, SigningPriv: sigPriv}
	copy(id.EncryptionPriv[:], priv.Bytes())
	copy(id.EncryptionPub[:], priv.PublicKey().Bytes())
	id.Fingerprint = keyFingerprint(id.EncryptionPub[:])
	return id, nil
}

// LockedIdentity is an Identity's private keys encrypted under an Argon2id-derived key.
// This is the at-rest form stored in Vault.
type LockedIdentity struct {
	Salt       []byte `json:"salt"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

// lockedKeys is the plaintext that gets sealed inside a LockedIdentity.
type lockedKeys struct {
	EncryptionPriv []byte `json:"enc_priv"`
	SigningPriv    []byte `json:"sig_priv,omitempty"`
}

// LockIdentity encrypts the identity's private keys under password.
func LockIdentity(id *Identity, password []byte) (*LockedIdentity, error) {
	salt, err := GenerateSalt()
	if err != nil {
		return nil, fmt.Errorf("generating salt: %w", err)
	}
	keys := lockedKeys{EncryptionPriv: id.EncryptionPriv[:]}
	if len(id.SigningPriv) > 0 {
		keys.SigningPriv = id.SigningPriv
	}
	plaintext, err := json.Marshal(keys)
	if err != nil {
		return nil, fmt.Errorf("marshaling key material: %w", err)
	}
	encKey := DeriveUnlockKey(password, salt)
	nonce, ciphertext, err := Seal(encKey, plaintext, nil)
	if err != nil {
		return nil, fmt.Errorf("sealing key material: %w", err)
	}
	return &LockedIdentity{Salt: salt, Nonce: nonce, Ciphertext: ciphertext}, nil
}

// UnlockIdentity decrypts a LockedIdentity with password and reconstructs the Identity.
// Returns a generic error on wrong password to avoid leaking information.
func UnlockIdentity(locked *LockedIdentity, password []byte) (*Identity, error) {
	encKey := DeriveUnlockKey(password, locked.Salt)
	plaintext, err := Open(encKey, locked.Nonce, locked.Ciphertext, nil)
	if err != nil {
		return nil, errors.New("incorrect password or corrupted key material")
	}
	var keys lockedKeys
	if err := json.Unmarshal(plaintext, &keys); err != nil {
		return nil, fmt.Errorf("parsing key material: %w", err)
	}
	id := &Identity{}
	copy(id.EncryptionPriv[:], keys.EncryptionPriv)
	if len(keys.SigningPriv) > 0 {
		id.SigningPriv = ed25519.PrivateKey(keys.SigningPriv)
		id.SigningPub = id.SigningPriv.Public().(ed25519.PublicKey)
	}
	// Derive the public key from the private key so it is always consistent.
	priv, err := ecdh.X25519().NewPrivateKey(id.EncryptionPriv[:])
	if err != nil {
		return nil, fmt.Errorf("parsing encryption private key: %w", err)
	}
	copy(id.EncryptionPub[:], priv.PublicKey().Bytes())
	id.Fingerprint = keyFingerprint(id.EncryptionPub[:])
	return id, nil
}

// EnsureSigningKey attaches a freshly generated Ed25519 signing keypair if the
// identity has none — the migration path for identities created before signing
// keys existed. It reports whether a key was added, so the caller knows to
// persist and re-publish.
func (id *Identity) EnsureSigningKey() (bool, error) {
	if len(id.SigningPriv) == ed25519.PrivateKeySize {
		return false, nil
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return false, fmt.Errorf("generating Ed25519 key: %w", err)
	}
	id.SigningPub = pub
	id.SigningPriv = priv
	return true, nil
}

func keyFingerprint(pub []byte) string {
	h := sha256.Sum256(pub)
	return hex.EncodeToString(h[:])
}