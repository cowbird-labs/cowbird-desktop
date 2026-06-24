package vault

import (
	"context"

	"cowbird/internal/crypto"
)

// GetOrganization retrieves the user's encrypted organization overlay (favorites
// and labels). Returns sharing.ErrNotFound when none has been stored yet.
// Vault only ever sees the sealed blob; the plaintext record stays on the client.
func (v *Vault) GetOrganization(ctx context.Context) (*crypto.SelfSealed, error) {
	var sealed crypto.SelfSealed
	if _, err := v.kvRead(ctx, "users/"+v.EntityID+"/organization", &sealed); err != nil {
		return nil, err
	}
	return &sealed, nil
}

// PutOrganization stores the user's encrypted organization overlay.
func (v *Vault) PutOrganization(ctx context.Context, sealed *crypto.SelfSealed) error {
	_, err := v.kvWrite(ctx, "users/"+v.EntityID+"/organization", sealed)
	return err
}
