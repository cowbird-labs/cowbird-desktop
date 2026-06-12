package auth

import (
	"context"
	"errors"
	"fmt"

	"cowbird/internal/credentials"

	vault "github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
)

type Token struct{}

func (t *Token) Name() string { return "Token" }

func (t *Token) Fields() []Field {
	return []Field{
		{Key: "token", Label: "Token", Secret: true},
	}
}

func (t *Token) Validate(values map[string]string) error {
	if values["token"] == "" {
		return errors.New("token is required")
	}
	return nil
}

func (t *Token) Authenticate(client *vault.Client, store credentials.CredentialStore) (Result, error) {
	token, err := store.Get("token")
	if err != nil {
		return Result{}, fmt.Errorf("retrieving token: %w", err)
	}
	// Static token auth: the token itself is the credential. We do a
	// lookup to validate it and retrieve the entity ID.
	if err := client.SetToken(token); err != nil {
		return Result{}, fmt.Errorf("setting token: %w", err)
	}

	resp, err := client.Auth.TokenLookUpSelf(context.Background())
	if err != nil {
		return Result{}, fmt.Errorf("validating token: %w", err)
	}

	entityID, _ := resp.Data["entity_id"].(string)
	displayName, _ := resp.Data["display_name"].(string)
	return Result{Token: token, EntityID: entityID, DisplayName: displayName}, nil
}

func (t *Token) Renew(client *vault.Client, store credentials.CredentialStore, token string) (Result, error) {
	if err := client.SetToken(token); err != nil {
		return Result{}, fmt.Errorf("setting token: %w", err)
	}

	resp, err := client.Auth.TokenRenewSelf(
		context.Background(),
		schema.TokenRenewSelfRequest{},
	)
	if err != nil {
		// Token auth has no re-auth path -- surface the error clearly.
		return Result{}, fmt.Errorf("token renewal failed and token auth cannot re-authenticate automatically: %w", err)
	}

	entityID, _ := resp.Auth.EntityID, "" // EntityID doesn't change on renewal
	_ = entityID
	return Result{Token: resp.Auth.ClientToken, EntityID: resp.Auth.EntityID}, nil
}
