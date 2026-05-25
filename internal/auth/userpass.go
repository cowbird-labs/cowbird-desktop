package auth

import (
	"context"
	"errors"
	"fmt"

	"cowbird/internal/credentials"

	vault "github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
)

type Userpass struct{}

func (u *Userpass) Name() string { return "Username & Password" }

func (u *Userpass) Fields() []Field {
	return []Field{
		{Key: "username", Label: "Username", Secret: false},
		{Key: "password", Label: "Password", Secret: true},
	}
}

func (u *Userpass) Validate(values map[string]string) error {
	if values["username"] == "" {
		return errors.New("username is required")
	}
	if values["password"] == "" {
		return errors.New("password is required")
	}
	return nil
}

func (u *Userpass) Authenticate(client *vault.Client, store credentials.CredentialStore) (Result, error) {
	username, err := store.Get("username")
	if err != nil {
		return Result{}, fmt.Errorf("retrieving username: %w", err)
	}
	password, err := store.Get("password")
	if err != nil {
		return Result{}, fmt.Errorf("retrieving password: %w", err)
	}

	resp, err := client.Auth.UserpassLogin(
		context.Background(),
		username,
		schema.UserpassLoginRequest{Password: password},
	)
	if err != nil {
		return Result{}, fmt.Errorf("logging in: %w", err)
	}

	return Result{
		Token:    resp.Auth.ClientToken,
		EntityID: resp.Auth.EntityID,
	}, nil
}

func (u *Userpass) Renew(client *vault.Client, store credentials.CredentialStore, token string) (Result, error) {
	if err := client.SetToken(token); err != nil {
		return u.Authenticate(client, store)
	}

	resp, err := client.Auth.TokenRenewSelf(
		context.Background(),
		schema.TokenRenewSelfRequest{},
	)
	if err != nil {
		return u.Authenticate(client, store)
	}

	return Result{
		Token:    resp.Auth.ClientToken,
		EntityID: resp.Auth.EntityID,
	}, nil
}
