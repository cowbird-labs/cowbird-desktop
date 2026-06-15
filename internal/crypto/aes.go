package crypto

import (
	"crypto/rand"
	"errors"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

// Seal encrypts plaintext with XChaCha20-Poly1305 using key. aad is
// authenticated but not encrypted (pass nil when no associated data is needed);
// the same aad must be supplied to Open. Returns the randomly generated nonce
// and the ciphertext+tag.
func Seal(key, plaintext, aad []byte) (nonce, ciphertext []byte, err error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, nil, fmt.Errorf("new cipher: %w", err)
	}
	nonce = make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("random nonce: %w", err)
	}
	return nonce, aead.Seal(nil, nonce, plaintext, aad), nil
}

// Open decrypts XChaCha20-Poly1305 ciphertext, authenticating aad (which must
// match what was passed to Seal). Returns a generic error on failure to avoid
// leaking whether the failure was due to a wrong key, wrong aad, or tampering.
func Open(key, nonce, ciphertext, aad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, errors.New("decryption failed")
	}
	return plaintext, nil
}
