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
	// Finish a key rotation interrupted before completion, before the identity
	// is handed back and the app becomes usable. After this, every owned item is
	// wrapped to the canonical key.
	if err := completeInterruptedRotation(ctx, v, id, password); err != nil {
		return nil, fmt.Errorf("completing key rotation: %w", err)
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

// RotateKey rotates the user's encryption keypair for compromise recovery.
// A new keypair is generated; every owned item is re-encrypted under a fresh
// item key wrapped to it, outstanding shares are re-distributed to recipients'
// current keys, the new public key is published, and the old keypair is
// destroyed. password is verified against, and used to re-lock, the identity.
//
// The operation is staged so an interruption is recoverable: the old key is
// written to a transitional slot before the new key becomes canonical, and the
// slot is removed only once every owned item is migrated. A rotation already in
// progress (transitional slot present) is finished rather than restarted.
func RotateKey(ctx context.Context, app *App, password []byte) error {
	v := app.Vault

	locked, err := v.GetLockedIdentity(ctx)
	if err != nil {
		return fmt.Errorf("loading identity: %w", err)
	}
	canonical, err := crypto.UnlockIdentity(locked, password)
	if err != nil {
		return err
	}

	// A transitional slot means a prior rotation did not finish; the canonical
	// identity is already the new one, so just complete it.
	if _, err := v.GetPrevLockedIdentity(ctx); err == nil {
		if err := completeInterruptedRotation(ctx, v, canonical, password); err != nil {
			return err
		}
		app.adoptIdentity(canonical)
		return nil
	} else if !errors.Is(err, sharing.ErrNotFound) {
		return fmt.Errorf("checking rotation state: %w", err)
	}

	newID, err := crypto.NewIdentity()
	if err != nil {
		return fmt.Errorf("generating keypair: %w", err)
	}
	// Stage the old key first, then make the new key canonical. A crash between
	// these leaves canonical and prev both holding the old key (same
	// fingerprint), which the resume path recognises and cleans up.
	if err := v.PutPrevLockedIdentity(ctx, locked); err != nil {
		return fmt.Errorf("staging prior key: %w", err)
	}
	newLocked, err := crypto.LockIdentity(newID, password)
	if err != nil {
		return fmt.Errorf("locking new identity: %w", err)
	}
	if err := v.PutLockedIdentity(ctx, newLocked); err != nil {
		return fmt.Errorf("storing new identity: %w", err)
	}

	if err := completeInterruptedRotation(ctx, v, newID, password); err != nil {
		return err
	}
	app.adoptIdentity(newID)
	return nil
}

// completeInterruptedRotation finishes a rotation if the transitional slot is
// present. canonical must be the already-unlocked canonical identity (the new
// keypair when a rotation is mid-flight). It is a no-op when no rotation is in
// progress, and idempotent so it can run at every unlock.
func completeInterruptedRotation(ctx context.Context, v *vault.Vault, canonical *crypto.Identity, password []byte) error {
	prevLocked, err := v.GetPrevLockedIdentity(ctx)
	if errors.Is(err, sharing.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("loading prior identity: %w", err)
	}
	oldID, err := crypto.UnlockIdentity(prevLocked, password)
	if err != nil {
		return fmt.Errorf("unlocking prior identity: %w", err)
	}

	// Aborted before the new key was committed: canonical and prev are the same
	// keypair, nothing was migrated. Discard the stale slot and stop.
	if oldID.Fingerprint == canonical.Fingerprint {
		return v.DeletePrevLockedIdentity(ctx)
	}

	// Publish the new public key so future shares target it, then re-key every
	// owned item and re-distribute shares using the old key to read.
	if err := v.PutPublicKey(ctx, v.EntityID, canonical.EncryptionPub, v.DisplayName); err != nil {
		return fmt.Errorf("publishing rotated public key: %w", err)
	}
	svc := sharing.NewService(v.EntityID, canonical, v)
	if err := svc.Rekey(ctx, oldID.EncryptionPriv, canonical.EncryptionPriv, canonical.EncryptionPub); err != nil {
		return fmt.Errorf("re-keying items: %w", err)
	}
	// All owned items migrated: destroy the old key.
	return v.DeletePrevLockedIdentity(ctx)
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