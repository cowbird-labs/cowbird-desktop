package sharing

import (
	"crypto/rand"
	"encoding/json"
	"fmt"

	"cowbird/internal/crypto"
	"cowbird/internal/items"
)

// contentFormatAAD marks an envelope whose content ciphertext authenticates the
// associated data from contentAAD. Format 0 (absent) is the legacy form sealed
// with nil AAD.
const contentFormatAAD = 1

// contentAAD is the associated data binding item content to its envelope. It
// covers only fields that are identical across the owner's copy and every shared
// copy — OwnerID and Type — so the single shared ciphertext still opens under
// each copy. ID is deliberately excluded: shared copies reuse the owner's
// ciphertext under a different ID (the shareID), so binding ID would break them.
func contentAAD(ownerID string, typ items.ItemType) []byte {
	aad := make([]byte, 0, len(ownerID)+1+len(typ))
	aad = append(aad, ownerID...)
	aad = append(aad, 0) // separator; neither field contains a NUL
	aad = append(aad, typ...)
	return aad
}

// envelopeAAD returns the associated data to authenticate when opening env,
// honoring its Format so legacy (nil-AAD) envelopes still decrypt.
func envelopeAAD(env Envelope) []byte {
	if env.Format < contentFormatAAD {
		return nil
	}
	return contentAAD(env.OwnerID, env.Type)
}

// NewEnvelope creates an encrypted Envelope for content owned by ownerID.
// The owner's wrapped item key is placed in Recipients[0].
// The plaintext item key is also returned so the caller can wrap it for
// additional recipients without decrypting the envelope again.
func NewEnvelope(ownerID string, ownerPub [32]byte, content items.Content) (Envelope, []byte, error) {
	itemKey, err := crypto.NewItemKey()
	if err != nil {
		return Envelope{}, nil, fmt.Errorf("generating item key: %w", err)
	}

	contentBytes, err := items.Encode(content)
	if err != nil {
		return Envelope{}, nil, fmt.Errorf("encoding content: %w", err)
	}

	nonce, ciphertext, err := crypto.Seal(itemKey, contentBytes, contentAAD(ownerID, content.Kind()))
	if err != nil {
		return Envelope{}, nil, fmt.Errorf("sealing content: %w", err)
	}

	ownerWK, err := WrapKeyForRecipient(itemKey, ownerID, ownerPub)
	if err != nil {
		return Envelope{}, nil, fmt.Errorf("wrapping key for owner: %w", err)
	}

	env := Envelope{
		ID:         newID(),
		Type:       content.Kind(),
		OwnerID:    ownerID,
		Format:     contentFormatAAD,
		Recipients: []WrappedKey{ownerWK},
		Nonce:      nonce,
		Ciphertext: ciphertext,
	}
	return env, itemKey, nil
}

// OpenEnvelope decrypts an Envelope using the recipient's private key and their WrappedKey.
func OpenEnvelope(env Envelope, recipientPriv [32]byte, wk WrappedKey) (items.Content, error) {
	itemKey, err := crypto.UnwrapKey(recipientPriv, wk.EphemeralPub, wk.Nonce, wk.Wrapped)
	if err != nil {
		return nil, fmt.Errorf("unwrapping item key: %w", err)
	}
	contentBytes, err := crypto.Open(itemKey, env.Nonce, env.Ciphertext, envelopeAAD(env))
	if err != nil {
		return nil, fmt.Errorf("decrypting content: %w", err)
	}
	return items.Decode(contentBytes)
}

// WrapKeyForRecipient wraps itemKey to a specific recipient's X25519 public key.
func WrapKeyForRecipient(itemKey []byte, recipientID string, recipientPub [32]byte) (WrappedKey, error) {
	ephPub, nonce, wrapped, err := crypto.WrapKey(recipientPub, itemKey)
	if err != nil {
		return WrappedKey{}, fmt.Errorf("wrapping key for %s: %w", recipientID, err)
	}
	return WrappedKey{
		RecipientID:  recipientID,
		EphemeralPub: ephPub,
		Nonce:        nonce,
		Wrapped:      wrapped,
	}, nil
}

// findOwnerKey returns the owner's WrappedKey from the envelope's Recipients list.
func findOwnerKey(env Envelope, ownerID string) (WrappedKey, bool) {
	for _, wk := range env.Recipients {
		if wk.RecipientID == ownerID {
			return wk, true
		}
	}
	return WrappedKey{}, false
}

// marshalWrappedKey serializes a WrappedKey to JSON bytes for storage in
// SharePayload.WrappedKey and SharedLink.WrappedKey.
func marshalWrappedKey(wk WrappedKey) ([]byte, error) {
	b, err := json.Marshal(wk)
	if err != nil {
		return nil, fmt.Errorf("marshaling wrapped key: %w", err)
	}
	return b, nil
}

// unmarshalWrappedKey deserializes a WrappedKey from the bytes stored in
// SharePayload.WrappedKey or SharedLink.WrappedKey.
func unmarshalWrappedKey(b []byte) (WrappedKey, error) {
	var wk WrappedKey
	if err := json.Unmarshal(b, &wk); err != nil {
		return WrappedKey{}, fmt.Errorf("parsing wrapped key: %w", err)
	}
	return wk, nil
}

// newID returns a random UUID v4.
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}