package core

import (
	"context"
	"errors"
	"fmt"

	"cowbird/internal/crypto"
	"cowbird/internal/organization"
	"cowbird/internal/sharing"
)

// LoadOrganization retrieves and decrypts the user's organization overlay
// (favorites and labels). A user who has never saved organization yet gets a
// fresh empty record rather than an error.
func (a *App) LoadOrganization(ctx context.Context) (*organization.Organization, error) {
	sealed, err := a.Vault.GetOrganization(ctx)
	if errors.Is(err, sharing.ErrNotFound) {
		return organization.New(), nil
	}
	if err != nil {
		return nil, err
	}
	plaintext, err := crypto.OpenFromSelf(a.Identity, sealed)
	if err != nil {
		return nil, fmt.Errorf("decrypting organization: %w", err)
	}
	return organization.ParseOrganization(plaintext)
}

// SaveOrganization encrypts and stores the user's organization overlay.
func (a *App) SaveOrganization(ctx context.Context, o *organization.Organization) error {
	plaintext, err := o.JSON()
	if err != nil {
		return fmt.Errorf("serializing organization: %w", err)
	}
	sealed, err := crypto.SealToSelf(a.Identity, plaintext)
	if err != nil {
		return fmt.Errorf("encrypting organization: %w", err)
	}
	return a.Vault.PutOrganization(ctx, sealed)
}
