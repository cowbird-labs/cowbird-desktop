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
	// Migrate identities created before share-signing keys existed (008): mint
	// one and persist it under the same password, so this user can sign shares
	// and publish a signing key. Done before re-publishing below.
	if added, err := id.EnsureSigningKey(); err != nil {
		return nil, fmt.Errorf("ensuring signing key: %w", err)
	} else if added {
		relocked, err := crypto.LockIdentity(id, password)
		if err != nil {
			return nil, fmt.Errorf("re-locking with signing key: %w", err)
		}
		if err := v.PutLockedIdentity(ctx, relocked); err != nil {
			return nil, fmt.Errorf("storing identity with signing key: %w", err)
		}
	}
	// Finish a key rotation interrupted before completion, before the identity
	// is handed back and the app becomes usable. After this, every owned item is
	// wrapped to the canonical key.
	if err := completeInterruptedRotation(ctx, v, id, password); err != nil {
		return nil, fmt.Errorf("completing key rotation: %w", err)
	}
	// Re-publish the public key so the directory entry carries the current
	// display name (entries published before names existed self-heal here) and
	// the signing key (entries published before 008 self-heal here).
	if err := v.PutPublicKey(ctx, v.EntityID, id.EncryptionPub, id.SigningPub, v.DisplayName); err != nil {
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
	if err := v.PutPublicKey(ctx, v.EntityID, canonical.EncryptionPub, canonical.SigningPub, v.DisplayName); err != nil {
		return fmt.Errorf("publishing rotated public key: %w", err)
	}
	svc := sharing.NewService(v.EntityID, canonical, v)
	if err := svc.Rekey(ctx, oldID.EncryptionPriv, canonical.EncryptionPriv, canonical.EncryptionPub); err != nil {
		return fmt.Errorf("re-keying items: %w", err)
	}
	// All owned items migrated: destroy the old key.
	return v.DeletePrevLockedIdentity(ctx)
}

// ErrIdentityMismatch is returned by ImportIdentity when the recovery file's
// keypair does not match the user's published public key — importing it would
// overwrite the identity their items are wrapped to. The caller may retry with
// force=true to override after confirming with the user.
var ErrIdentityMismatch = errors.New("recovery file is for a different identity")

// ImportIdentity restores a keypair from a passphrase-protected recovery file
// and installs it as the Vault-stored locked identity, re-locked under a new
// unlock password. It is the recovery counterpart to ExportIdentity, run from
// the unlock window when the user cannot sign in normally.
//
// Before overwriting an existing identity it compares the recovered public key
// with the user's published public key; on mismatch it returns
// ErrIdentityMismatch unless force is set. The private key is persisted only in
// locked form. Any stale key-rotation marker is cleared, since the imported
// identity is a fresh starting point.
func ImportIdentity(ctx context.Context, v *vault.Vault, data, exportPassphrase, newUnlockPassword []byte, force bool) (*crypto.Identity, error) {
	id, err := crypto.ImportKey(data, exportPassphrase)
	if err != nil {
		return nil, err
	}
	// A recovery file made before signing keys existed restores an identity with
	// none; mint one now so the imported session can sign shares immediately and
	// publishes a signing key below.
	if _, err := id.EnsureSigningKey(); err != nil {
		return nil, fmt.Errorf("ensuring signing key: %w", err)
	}

	// Safety check: refuse to overwrite a different identity unless forced.
	switch existingPub, err := v.GetPublicKey(ctx, v.EntityID); {
	case errors.Is(err, sharing.ErrNotFound):
		// No published key (fresh/cleared Vault) — nothing to conflict with.
	case err != nil:
		return nil, fmt.Errorf("checking published key: %w", err)
	case existingPub != id.EncryptionPub && !force:
		return nil, ErrIdentityMismatch
	}

	locked, err := crypto.LockIdentity(id, newUnlockPassword)
	if err != nil {
		return nil, fmt.Errorf("locking identity: %w", err)
	}
	if err := v.PutLockedIdentity(ctx, locked); err != nil {
		return nil, fmt.Errorf("storing identity: %w", err)
	}
	// Clear any in-progress rotation marker: it is locked under the old unlock
	// password and would otherwise block the next unlock.
	if err := v.DeletePrevLockedIdentity(ctx); err != nil && !errors.Is(err, sharing.ErrNotFound) {
		return nil, fmt.Errorf("clearing rotation marker: %w", err)
	}
	if err := v.PutPublicKey(ctx, v.EntityID, id.EncryptionPub, id.SigningPub, v.DisplayName); err != nil {
		return nil, fmt.Errorf("publishing public key: %w", err)
	}
	return id, nil
}

// ExportIdentity produces a passphrase-protected recovery file for the user's
// keypair. It is gated behind the current unlock password: the stored locked
// identity is decrypted with unlockPassword (verifies it; generic error on
// mismatch), then the verified identity is exported under a separate
// exportPassphrase. The returned bytes are the file contents; writing them to a
// location is the caller's responsibility. Nothing is written to Vault, and the
// private key is never serialized except in the passphrase-encrypted form.
func ExportIdentity(ctx context.Context, app *App, unlockPassword, exportPassphrase []byte) ([]byte, error) {
	locked, err := app.Vault.GetLockedIdentity(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading identity: %w", err)
	}
	id, err := crypto.UnlockIdentity(locked, unlockPassword)
	if err != nil {
		return nil, err
	}
	data, err := crypto.ExportKey(id, exportPassphrase)
	if err != nil {
		return nil, fmt.Errorf("exporting key: %w", err)
	}
	return data, nil
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
	if err := v.PutPublicKey(ctx, v.EntityID, id.EncryptionPub, id.SigningPub, v.DisplayName); err != nil {
		return nil, fmt.Errorf("publishing public key: %w", err)
	}
	return id, nil
}