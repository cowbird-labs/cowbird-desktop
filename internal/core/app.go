package core

import (
	"cowbird/internal/crypto"
	"cowbird/internal/sharing"
	"cowbird/internal/vault"
)

// App is the fully initialised application state: an authenticated Vault
// connection, a decrypted identity, and a sharing service wired to both.
type App struct {
	Vault    *vault.Vault
	Identity *crypto.Identity
	Service  *sharing.Service
}

func NewApp(v *vault.Vault, id *crypto.Identity) *App {
	return &App{
		Vault:    v,
		Identity: id,
		Service:  sharing.NewService(v.EntityID, id, v),
	}
}

// adoptIdentity swaps in a new identity (after key rotation) and rebuilds the
// sharing service so subsequent operations use the new keypair.
func (a *App) adoptIdentity(id *crypto.Identity) {
	a.Identity = id
	a.Service = sharing.NewService(a.Vault.EntityID, id, a.Vault)
}