package core

import (
	"context"
	"errors"
	"fmt"

	"cowbird/internal/crypto"
	"cowbird/internal/sharing"
	"cowbird/internal/vault"
)

// InitIdentity creates or unlocks the user's identity.
//
// First use (no identity in Vault): generates a new X25519 keypair, encrypts
// it under password, stores it in Vault, and publishes the public key to the
// shared directory.
//
// Subsequent uses: retrieves the stored locked identity and decrypts it.
// Returns a generic error on wrong password (from crypto.UnlockIdentity) so
// the caller can display it without leaking information.
func InitIdentity(ctx context.Context, v *vault.Vault, password []byte) (*crypto.Identity, error) {
	locked, err := v.GetLockedIdentity(ctx)
	if errors.Is(err, sharing.ErrNotFound) {
		return createIdentity(ctx, v, password)
	}
	if err != nil {
		return nil, fmt.Errorf("loading identity: %w", err)
	}
	id, err := crypto.UnlockIdentity(locked, password)
	if err != nil {
		return nil, err
	}
	// Re-publish the public key so the directory entry carries the current
	// display name (entries published before names existed self-heal here).
	if err := v.PutPublicKey(ctx, v.EntityID, id.EncryptionPub, v.DisplayName); err != nil {
		return nil, fmt.Errorf("refreshing public key: %w", err)
	}
	return id, nil
}

// ChangePassword re-wraps the user's locked identity under a new unlock
// password. The underlying keypair is unchanged, so no item contents are
// re-encrypted and recipients' wrapped keys stay valid.
//
// The current password is verified by decrypting the stored identity; a wrong
// password yields the generic error from crypto.UnlockIdentity. On success the
// freshly unlocked identity is re-locked under a new Argon2id salt and written
// back in a single replacement write — a failure leaves the old record intact
// and still unlockable with the old password.
func ChangePassword(ctx context.Context, v *vault.Vault, oldPassword, newPassword []byte) error {
	locked, err := v.GetLockedIdentity(ctx)
	if errors.Is(err, sharing.ErrNotFound) {
		return errors.New("no identity to change the password for")
	}
	if err != nil {
		return fmt.Errorf("loading identity: %w", err)
	}
	id, err := crypto.UnlockIdentity(locked, oldPassword)
	if err != nil {
		return err
	}
	relocked, err := crypto.LockIdentity(id, newPassword)
	if err != nil {
		return fmt.Errorf("re-locking identity: %w", err)
	}
	if err := v.PutLockedIdentity(ctx, relocked); err != nil {
		return fmt.Errorf("storing identity: %w", err)
	}
	return nil
}

func createIdentity(ctx context.Context, v *vault.Vault, password []byte) (*crypto.Identity, error) {
	id, err := crypto.NewIdentity()
	if err != nil {
		return nil, fmt.Errorf("generating keypair: %w", err)
	}
	locked, err := crypto.LockIdentity(id, password)
	if err != nil {
		return nil, fmt.Errorf("locking identity: %w", err)
	}
	if err := v.PutLockedIdentity(ctx, locked); err != nil {
		return nil, fmt.Errorf("storing identity: %w", err)
	}
	if err := v.PutPublicKey(ctx, v.EntityID, id.EncryptionPub, v.DisplayName); err != nil {
		return nil, fmt.Errorf("publishing public key: %w", err)
	}
	return id, nil
}