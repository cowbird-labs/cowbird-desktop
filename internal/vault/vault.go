package vault

import (
	"context"
	"errors"
	"fmt"
	"log"
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
