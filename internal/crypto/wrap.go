package crypto

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

// WrapKey encrypts itemKey to the recipient's X25519 public key via an ephemeral
// ECDH exchange. Returns the ephemeral public key (32 bytes), nonce, and wrapped ciphertext.
func WrapKey(recipientPub [32]byte, itemKey []byte) (ephemeralPub, nonce, wrapped []byte, err error) {
	curve := ecdh.X25519()

	ephPriv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("generating ephemeral key: %w", err)
	}

	recipPub, err := curve.NewPublicKey(recipientPub[:])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("parsing recipient public key: %w", err)
	}

	shared, err := ephPriv.ECDH(recipPub)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ECDH: %w", err)
	}

	ephPub := ephPriv.PublicKey().Bytes()
	wrapKey := deriveWrapKey(shared, ephPub, recipientPub[:])

	nonce, wrapped, err = Seal(wrapKey, itemKey, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("sealing item key: %w", err)
	}

	return ephPub, nonce, wrapped, nil
}

// UnwrapKey decrypts a wrapped item key using the recipient's private key.
func UnwrapKey(recipientPriv [32]byte, ephemeralPub, nonce, wrapped []byte) ([]byte, error) {
	curve := ecdh.X25519()

	priv, err := curve.NewPrivateKey(recipientPriv[:])
	if err != nil {
		return nil, fmt.Errorf("parsing recipient private key: %w", err)
	}

	ephPub, err := curve.NewPublicKey(ephemeralPub)
	if err != nil {
		return nil, fmt.Errorf("parsing ephemeral public key: %w", err)
	}

	shared, err := priv.ECDH(ephPub)
	if err != nil {
		return nil, fmt.Errorf("ECDH: %w", err)
	}

	wrapKey := deriveWrapKey(shared, ephemeralPub, priv.PublicKey().Bytes())
	return Open(wrapKey, nonce, wrapped, nil)
}

// deriveWrapKey derives a 32-byte key from the ECDH shared secret via HKDF.
// Both public keys are mixed into the salt so the derived key is unique to this
// exchange and cannot be reused across different sender/recipient pairs.
func deriveWrapKey(shared, ephemeralPub, recipientPub []byte) []byte {
	salt := make([]byte, len(ephemeralPub)+len(recipientPub))
	copy(salt, ephemeralPub)
	copy(salt[len(ephemeralPub):], recipientPub)
	r := hkdf.New(sha256.New, shared, salt, []byte("cowbird-wrap-v1"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(r, key); err != nil {
		panic(err) // unreachable: HKDF does not fail with valid inputs
	}
	return key
}