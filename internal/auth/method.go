package auth

import (
	"cowbird/internal/credentials"

	vault "github.com/hashicorp/vault-client-go"
)

// Field describes a single credential input for an auth method.
type Field struct {
	Key    string
	Label  string
	Secret bool
}

// Result  holds the output of a successful authentication.
// DisplayName is the human-readable identity the auth backend reports
// (userpass username, token display name, AppRole role name); it may be
// empty, notably on renewal paths.
type Result struct {
	Token       string
	EntityID    string
	DisplayName string
}

// Method is the interface each auth backend must implement.
type Method interface {
	// Name returns a human-readable label shown in the UI picker.
	Name() string

	// Fields returns the credential inputs this method requires.
	Fields() []Field

	// Validate checks field values before any network call is made.
	Validate(values map[string]string) error

	// Authenticate performs a full login and returns an AuthResult.
	Authenticate(client *vault.Client, store credentials.CredentialStore) (Result, error)

	// Renew attempts to extend the current token. If the renewal
	// fails it falls back to a full Authenticate call.
	Renew(client *vault.Client, store credentials.CredentialStore, token string) (Result, error)
}
