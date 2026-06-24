package crypto

import (
	"errors"
	"fmt"
)

// SelfSealed is a blob encrypted to the holder's own X25519 key, decryptable
// with their in-memory private key (no password prompt). It is a single-
// recipient-to-self envelope: a random content key encrypts the plaintext and is
// itself wrapped to the owner's public key. Used for per-user metadata records
// (e.g. the organization overlay) that must stay private to the user and opaque
// to the storage operator.
type SelfSealed struct {
	EphemeralPub []byte `json:"ephemeral_pub"` // wrap ECDH ephemeral public key
	WrapNonce    []byte `json:"wrap_nonce"`    // nonce for the wrapped content key
	WrappedKey   []byte `json:"wrapped_key"`   // content key wrapped to own public key
	Nonce        []byte `json:"nonce"`         // content nonce
	Ciphertext   []byte `json:"ciphertext"`    // plaintext sealed under the content key
}

// SealToSelf encrypts plaintext to the identity's own X25519 public key.
func SealToSelf(id *Identity, plaintext []byte) (*SelfSealed, error) {
	if id == nil {
		return nil, errors.New("nil identity")
	}
	contentKey, err := NewItemKey()
	if err != nil {
		return nil, fmt.Errorf("generating content key: %w", err)
	}
	nonce, ciphertext, err := Seal(contentKey, plaintext, nil)
	if err != nil {
		return nil, fmt.Errorf("sealing content: %w", err)
	}
	ephPub, wrapNonce, wrapped, err := WrapKey(id.EncryptionPub, contentKey)
	if err != nil {
		return nil, fmt.Errorf("wrapping content key: %w", err)
	}
	return &SelfSealed{
		EphemeralPub: ephPub,
		WrapNonce:    wrapNonce,
		WrappedKey:   wrapped,
		Nonce:        nonce,
		Ciphertext:   ciphertext,
	}, nil
}

// OpenFromSelf decrypts a SelfSealed blob with the identity's private key.
// Returns a generic error on failure (wrong key or tampering).
func OpenFromSelf(id *Identity, b *SelfSealed) ([]byte, error) {
	if id == nil {
		return nil, errors.New("nil identity")
	}
	if b == nil {
		return nil, errors.New("nil sealed blob")
	}
	contentKey, err := UnwrapKey(id.EncryptionPriv, b.EphemeralPub, b.WrapNonce, b.WrappedKey)
	if err != nil {
		return nil, err
	}
	return Open(contentKey, b.Nonce, b.Ciphertext, nil)
}
