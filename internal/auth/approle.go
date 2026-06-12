package auth

import (
	"context"
	"errors"
	"fmt"

	"cowbird/internal/credentials"

	vault "github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
)

type AppRole struct{}

func (a *AppRole) Name() string { return "AppRole" }

func (a *AppRole) Fields() []Field {
	return []Field{
		{Key: "role_id", Label: "Role ID", Secret: false},
		{Key: "secret_id", Label: "Secret ID", Secret: true},
	}
}

func (a *AppRole) Validate(values map[string]string) error {
	if values["role_id"] == "" {
		return errors.New("role ID is required")
	}
	if values["secret_id"] == "" {
		return errors.New("secret ID is required")
	}
	return nil
}

func (a *AppRole) Authenticate(client *vault.Client, store credentials.CredentialStore) (Result, error) {
	roleID, err := store.Get("role_id")
	if err != nil {
		return Result{}, fmt.Errorf("retrieving role ID: %w", err)
	}
	secretID, err := store.Get("secret_id")
	if err != nil {
		return Result{}, fmt.Errorf("retrieving secret ID: %w", err)
	}

	resp, err := client.Auth.AppRoleLogin(
		context.Background(),
		schema.AppRoleLoginRequest{
			RoleId:   roleID,
			SecretId: secretID,
		},
	)
	if err != nil {
		return Result{}, fmt.Errorf("logging in: %w", err)
	}

	return Result{
		Token:       resp.Auth.ClientToken,
		EntityID:    resp.Auth.EntityID,
		DisplayName: resp.Auth.Metadata["role_name"],
	}, nil
}

func (a *AppRole) Renew(client *vault.Client, store credentials.CredentialStore, token string) (Result, error) {
	if err := client.SetToken(token); err != nil {
		return a.Authenticate(client, store)
	}

	resp, err := client.Auth.TokenRenewSelf(
		context.Background(),
		schema.TokenRenewSelfRequest{},
	)
	if err != nil {
		return a.Authenticate(client, store)
	}

	return Result{
		Token:    resp.Auth.ClientToken,
		EntityID: resp.Auth.EntityID,
	}, nil
}
