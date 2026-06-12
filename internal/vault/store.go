package vault

import (
	"context"
	"encoding/json"
	"fmt"

	vaultclient "github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"

	"cowbird/internal/sharing"
)

// Compile-time assertion that *Vault implements sharing.Store.
var _ sharing.Store = (*Vault)(nil)

// kvWrite JSON-serializes val and writes it to path under the configured mount.
// Returns the KV v2 version number assigned by the server.
func (v *Vault) kvWrite(ctx context.Context, path string, val interface{}) (int64, error) {
	data, err := json.Marshal(val)
	if err != nil {
		return 0, fmt.Errorf("marshaling for %q: %w", path, err)
	}
	resp, err := v.client.Secrets.KvV2Write(
		ctx, path,
		schema.KvV2WriteRequest{Data: map[string]interface{}{"v": string(data)}},
		vaultclient.WithMountPath(v.Mount),
	)
	if err != nil {
		return 0, err
	}
	if resp == nil {
		return 0, nil
	}
	return resp.Data.Version, nil
}

// kvRead reads from path and JSON-deserializes the value into dest.
// Returns the KV v2 version and sharing.ErrNotFound on 404.
func (v *Vault) kvRead(ctx context.Context, path string, dest interface{}) (int64, error) {
	resp, err := v.client.Secrets.KvV2Read(ctx, path, vaultclient.WithMountPath(v.Mount))
	if err != nil {
		if vaultclient.IsErrorStatus(err, 404) {
			return 0, sharing.ErrNotFound
		}
		return 0, err
	}
	if resp == nil {
		return 0, sharing.ErrNotFound
	}
	jsonStr, ok := resp.Data.Data["v"].(string)
	if !ok {
		return 0, fmt.Errorf("unexpected data format at %q", path)
	}
	if err := json.Unmarshal([]byte(jsonStr), dest); err != nil {
		return 0, fmt.Errorf("unmarshaling %q: %w", path, err)
	}
	return versionFromMetadata(resp.Data.Metadata), nil
}

// kvDelete permanently removes all versions of path.
// Returns sharing.ErrNotFound on 404, which callers may safely ignore for
// idempotent delete operations.
func (v *Vault) kvDelete(ctx context.Context, path string) error {
	_, err := v.client.Secrets.KvV2DeleteMetadataAndAllVersions(
		ctx, path, vaultclient.WithMountPath(v.Mount),
	)
	if err != nil {
		if vaultclient.IsErrorStatus(err, 404) {
			return sharing.ErrNotFound
		}
		return err
	}
	return nil
}

// kvList returns the keys immediately under path.
// Returns an empty slice (not an error) when no keys exist yet.
func (v *Vault) kvList(ctx context.Context, path string) ([]string, error) {
	resp, err := v.client.Secrets.KvV2List(ctx, path, vaultclient.WithMountPath(v.Mount))
	if err != nil {
		if vaultclient.IsErrorStatus(err, 404) {
			return nil, nil
		}
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}
	return resp.Data.Keys, nil
}

// versionFromMetadata extracts the monotonic KV v2 version from a metadata map.
// Numbers arrive as json.Number because the response decoder uses UseNumber().
func versionFromMetadata(metadata map[string]interface{}) int64 {
	if metadata == nil {
		return 0
	}
	n, ok := metadata["version"].(json.Number)
	if !ok {
		return 0
	}
	v, _ := n.Int64()
	return v
}

// --- own items (users/<entityID>/items/<itemID>) ------------------------------

func (v *Vault) PutItem(ctx context.Context, itemID string, env sharing.Envelope) error {
	_, err := v.kvWrite(ctx, v.itemPath(itemID), env)
	return err
}

func (v *Vault) GetItem(ctx context.Context, itemID string) (sharing.Envelope, error) {
	var env sharing.Envelope
	_, err := v.kvRead(ctx, v.itemPath(itemID), &env)
	return env, err
}

func (v *Vault) DeleteItem(ctx context.Context, itemID string) error {
	return v.kvDelete(ctx, v.itemPath(itemID))
}

func (v *Vault) ListItems(ctx context.Context) ([]sharing.Envelope, error) {
	keys, err := v.kvList(ctx, "users/"+v.EntityID+"/items")
	if err != nil {
		return nil, err
	}
	envs := make([]sharing.Envelope, 0, len(keys))
	for _, key := range keys {
		env, err := v.GetItem(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("reading item %s: %w", key, err)
		}
		envs = append(envs, env)
	}
	return envs, nil
}

func (v *Vault) itemPath(itemID string) string {
	return "users/" + v.EntityID + "/items/" + itemID
}

// --- public-key directory (pubkeys/<entityID>) --------------------------------

func (v *Vault) GetPublicKey(ctx context.Context, entityID string) ([32]byte, error) {
	var rec pubkeyRecord
	if _, err := v.kvRead(ctx, "pubkeys/"+entityID, &rec); err != nil {
		return [32]byte{}, err
	}
	var pub [32]byte
	copy(pub[:], rec.Pub)
	return pub, nil
}

func (v *Vault) PutPublicKey(ctx context.Context, entityID string, pub [32]byte, name string) error {
	_, err := v.kvWrite(ctx, "pubkeys/"+entityID, pubkeyRecord{Pub: pub[:], Name: name})
	return err
}

func (v *Vault) ListPublicKeys(ctx context.Context) ([]sharing.PublicKeyEntry, error) {
	keys, err := v.kvList(ctx, "pubkeys")
	if err != nil {
		return nil, err
	}
	entries := make([]sharing.PublicKeyEntry, 0, len(keys))
	for _, entityID := range keys {
		var rec pubkeyRecord
		if _, err := v.kvRead(ctx, "pubkeys/"+entityID, &rec); err != nil {
			return nil, fmt.Errorf("reading pubkey %s: %w", entityID, err)
		}
		entry := sharing.PublicKeyEntry{EntityID: entityID, Name: rec.Name}
		copy(entry.Pub[:], rec.Pub)
		entries = append(entries, entry)
	}
	return entries, nil
}

// pubkeyRecord is the at-rest form of a user's X25519 public key.
// Name was added in 003; records published before it unmarshal with an
// empty name.
type pubkeyRecord struct {
	Pub  []byte `json:"pub"` // 32-byte X25519 public key, JSON-encoded as base64
	Name string `json:"name,omitempty"`
}

// --- shared envelopes (shared/<ownerEntityID>/<shareID>) ---------------------

func (v *Vault) PutSharedEnvelope(ctx context.Context, shareID string, env sharing.Envelope) (int64, error) {
	return v.kvWrite(ctx, v.sharedPath(shareID), env)
}

func (v *Vault) GetSharedEnvelope(ctx context.Context, ownerID, shareID string) (sharing.Envelope, int64, error) {
	var env sharing.Envelope
	version, err := v.kvRead(ctx, "shared/"+ownerID+"/"+shareID, &env)
	return env, version, err
}

func (v *Vault) DeleteSharedEnvelope(ctx context.Context, shareID string) error {
	return v.kvDelete(ctx, v.sharedPath(shareID))
}

func (v *Vault) sharedPath(shareID string) string {
	return "shared/" + v.EntityID + "/" + shareID
}

// --- inbox (inbox/<recipientEntityID>/<msgID>) --------------------------------

func (v *Vault) SendMessage(ctx context.Context, recipientID, msgID string, msg sharing.Message) error {
	_, err := v.kvWrite(ctx, "inbox/"+recipientID+"/"+msgID, msg)
	return err
}

func (v *Vault) ListInboxMessages(ctx context.Context) ([]sharing.InboxEntry, error) {
	keys, err := v.kvList(ctx, "inbox/"+v.EntityID)
	if err != nil {
		return nil, err
	}
	entries := make([]sharing.InboxEntry, 0, len(keys))
	for _, key := range keys {
		var msg sharing.Message
		if _, err := v.kvRead(ctx, "inbox/"+v.EntityID+"/"+key, &msg); err != nil {
			return nil, fmt.Errorf("reading inbox message %s: %w", key, err)
		}
		entries = append(entries, sharing.InboxEntry{ID: key, Msg: msg})
	}
	return entries, nil
}

func (v *Vault) DeleteInboxMessage(ctx context.Context, msgID string) error {
	return v.kvDelete(ctx, "inbox/"+v.EntityID+"/"+msgID)
}

// --- shared links (users/<entityID>/links/<shareID>) -------------------------

func (v *Vault) PutSharedLink(ctx context.Context, link sharing.SharedLink) error {
	_, err := v.kvWrite(ctx, v.linkPath(link.ShareID), link)
	return err
}

func (v *Vault) GetSharedLink(ctx context.Context, shareID string) (sharing.SharedLink, error) {
	var link sharing.SharedLink
	_, err := v.kvRead(ctx, v.linkPath(shareID), &link)
	return link, err
}

func (v *Vault) DeleteSharedLink(ctx context.Context, shareID string) error {
	return v.kvDelete(ctx, v.linkPath(shareID))
}

func (v *Vault) ListSharedLinks(ctx context.Context) ([]sharing.SharedLink, error) {
	keys, err := v.kvList(ctx, "users/"+v.EntityID+"/links")
	if err != nil {
		return nil, err
	}
	links := make([]sharing.SharedLink, 0, len(keys))
	for _, key := range keys {
		link, err := v.GetSharedLink(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("reading link %s: %w", key, err)
		}
		links = append(links, link)
	}
	return links, nil
}

func (v *Vault) linkPath(shareID string) string {
	return "users/" + v.EntityID + "/links/" + shareID
}

// --- share records (users/<entityID>/shares/<shareID>) ------------------------

func (v *Vault) PutShareRecord(ctx context.Context, rec sharing.ShareRecord) error {
	_, err := v.kvWrite(ctx, v.shareRecordPath(rec.ShareID), rec)
	return err
}

func (v *Vault) ListShareRecords(ctx context.Context) ([]sharing.ShareRecord, error) {
	keys, err := v.kvList(ctx, "users/"+v.EntityID+"/shares")
	if err != nil {
		return nil, err
	}
	recs := make([]sharing.ShareRecord, 0, len(keys))
	for _, key := range keys {
		var rec sharing.ShareRecord
		if _, err := v.kvRead(ctx, v.shareRecordPath(key), &rec); err != nil {
			return nil, fmt.Errorf("reading share record %s: %w", key, err)
		}
		recs = append(recs, rec)
	}
	return recs, nil
}

func (v *Vault) DeleteShareRecord(ctx context.Context, shareID string) error {
	return v.kvDelete(ctx, v.shareRecordPath(shareID))
}

func (v *Vault) shareRecordPath(shareID string) string {
	return "users/" + v.EntityID + "/shares/" + shareID
}