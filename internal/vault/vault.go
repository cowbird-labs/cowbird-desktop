package vault

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"time"

	"cowbird/internal/auth"
	"cowbird/internal/config"
	"cowbird/internal/credentials"

	vaultclient "github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
)

type Vault struct {
	client      *vaultclient.Client
	method      auth.Method
	creds       credentials.CredentialStore
	Mount       string
	EntityID    string
	DisplayName string // human-readable name from the auth backend; may be empty
	token       string
	cancel      context.CancelFunc
}

func NewVault(
	cfg config.Vault,
	creds credentials.CredentialStore,
	method auth.Method,
) (*Vault, error) {
	if err := validateAddress(cfg.Address); err != nil {
		return nil, err
	}

	client, err := vaultclient.New(
		vaultclient.WithAddress(cfg.Address),
		vaultclient.WithRequestTimeout(cfg.RequestTimeout),
	)
	if err != nil {
		return nil, fmt.Errorf("creating vault client: %w", err)
	}

	v := &Vault{
		client: client,
		method: method,
		creds:  creds,
		Mount:  cfg.MountPath,
	}

	result, err := v.authenticate()
	if err != nil {
		return nil, fmt.Errorf("initial authentication: %w", err)
	}
	v.EntityID = result.EntityID
	v.DisplayName = result.DisplayName

	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	go v.renewalLoop(ctx)

	return v, nil
}

// validateAddress rejects a Vault address that would send the auth token and KV
// traffic in cleartext. The at-rest item model is end-to-end encrypted, but the
// bearer token itself rides on the connection, so plain http would leak it.
// https is required; http is tolerated only for a loopback host (local dev).
func validateAddress(address string) error {
	u, err := url.Parse(address)
	if err != nil {
		return fmt.Errorf("invalid vault address %q: %w", address, err)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		host := u.Hostname()
		if host == "localhost" {
			return nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil
		}
		return fmt.Errorf("insecure vault address %q: http is only permitted for localhost; use https", address)
	default:
		return fmt.Errorf("invalid vault address %q: scheme must be https", address)
	}
}

// VerifyMount confirms the given mount path exists and is accessible
// with the token currently set on the client.
func VerifyMount(client *vaultclient.Client, mount string, entityID string) error {
	_, err := client.Secrets.KvV2List(
		context.Background(),
		"users/"+entityID,
		vaultclient.WithMountPath(mount),
	)
	if err == nil {
		return nil
	}
	// A 404 means no secrets exist yet -- mount is accessible and policy works.
	var respErr *vaultclient.ResponseError
	if errors.As(err, &respErr) && respErr.StatusCode == 404 {
		return nil
	}
	return fmt.Errorf("mount %q not accessible: %w", mount, err)
}

// IsUnreachable reports whether err indicates the Vault server could not be
// reached at all (network/DNS/TLS failure, connection refused, timeout), as
// opposed to the server responding with an error (e.g. bad credentials). A
// *vaultclient.ResponseError means the server answered, so it is reachable.
func IsUnreachable(err error) bool {
	if err == nil {
		return false
	}
	var respErr *vaultclient.ResponseError
	if errors.As(err, &respErr) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var urlErr *url.Error
	return errors.As(err, &urlErr)
}

// Close stops the background renewal loop.
func (v *Vault) Close() {
	if v.cancel != nil {
		v.cancel()
	}
}

func (v *Vault) authenticate() (auth.Result, error) {
	result, err := v.method.Authenticate(v.client, v.creds)
	if err != nil {
		return auth.Result{}, err
	}
	if err := v.client.SetToken(result.Token); err != nil {
		return auth.Result{}, fmt.Errorf("setting token: %w", err)
	}

	v.token = result.Token
	return result, nil
}

func (v *Vault) renewalLoop(ctx context.Context) {
	// Renew every 10 minutes; adjust based on your token TTL policy.
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current := v.token
			result, err := v.method.Renew(v.client, v.creds, current)
			if err != nil {
				log.Printf("vault: token renewal failed: %v", err)
				continue
			}
			if err := v.client.SetToken(result.Token); err != nil {
				log.Printf("vault: failed to set renewed token: %v", err)
				continue
			}
			if result.EntityID != "" {
				v.EntityID = result.EntityID
			}
		}
	}
}

func (v *Vault) Put(path string, data map[string]interface{}) error {
	_, err := v.client.Secrets.KvV2Write(
		context.Background(),
		"users/"+v.EntityID+"/"+path,
		schema.KvV2WriteRequest{Data: data},
		vaultclient.WithMountPath(v.Mount),
	)
	return err
}
