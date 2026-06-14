package vault

import (
	"context"

	"cowbird/internal/crypto"
)

// GetLockedIdentity retrieves the user's encrypted private key from Vault.
// Returns sharing.ErrNotFound if no identity has been stored yet (first run).
func (v *Vault) GetLockedIdentity(ctx context.Context) (*crypto.LockedIdentity, error) {
	var locked crypto.LockedIdentity
	if _, err := v.kvRead(ctx, "users/"+v.EntityID+"/identity", &locked); err != nil {
		return nil, err
	}
	return &locked, nil
}

// PutLockedIdentity stores the user's encrypted private key in Vault.
func (v *Vault) PutLockedIdentity(ctx context.Context, locked *crypto.LockedIdentity) error {
	_, err := v.kvWrite(ctx, "users/"+v.EntityID+"/identity", locked)
	return err
}

// GetPrevLockedIdentity retrieves the transitional prior identity written
// during key rotation. Its presence signals a rotation in progress; it holds
// the old keypair so an interrupted rotation can finish re-keying.
// Returns sharing.ErrNotFound when no rotation is in progress.
func (v *Vault) GetPrevLockedIdentity(ctx context.Context) (*crypto.LockedIdentity, error) {
	var locked crypto.LockedIdentity
	if _, err := v.kvRead(ctx, "users/"+v.EntityID+"/identity.prev", &locked); err != nil {
		return nil, err
	}
	return &locked, nil
}

// PutPrevLockedIdentity stages the old identity at the start of rotation.
func (v *Vault) PutPrevLockedIdentity(ctx context.Context, locked *crypto.LockedIdentity) error {
	_, err := v.kvWrite(ctx, "users/"+v.EntityID+"/identity.prev", locked)
	return err
}

// DeletePrevLockedIdentity removes the transitional prior identity once
// rotation completes — the point at which the old keypair is destroyed.
func (v *Vault) DeletePrevLockedIdentity(ctx context.Context) error {
	return v.kvDelete(ctx, "users/"+v.EntityID+"/identity.prev")
}