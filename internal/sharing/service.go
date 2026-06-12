package sharing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cowbird/internal/crypto"
	"cowbird/internal/items"
)

// Service coordinates item creation, sharing, and revocation for a single user.
type Service struct {
	entityID string
	identity *crypto.Identity
	store    Store
}

// NewService returns a Service for entityID backed by store.
// identity must be unlocked (EncryptionPriv populated).
func NewService(entityID string, identity *crypto.Identity, store Store) *Service {
	return &Service{entityID: entityID, identity: identity, store: store}
}

// CreateItem encrypts content and stores it in the owner's own subtree.
func (s *Service) CreateItem(ctx context.Context, content items.Content) (Envelope, error) {
	env, _, err := NewEnvelope(s.entityID, s.identity.EncryptionPub, content)
	if err != nil {
		return Envelope{}, fmt.Errorf("creating envelope: %w", err)
	}
	if err := s.store.PutItem(ctx, env.ID, env); err != nil {
		return Envelope{}, fmt.Errorf("storing item: %w", err)
	}
	return env, nil
}

// OpenOwnItem decrypts an item from the owner's own subtree.
func (s *Service) OpenOwnItem(_ context.Context, env Envelope) (items.Content, error) {
	wk, ok := findOwnerKey(env, s.entityID)
	if !ok {
		return nil, fmt.Errorf("no wrapped key for %s in item %s", s.entityID, env.ID)
	}
	return OpenEnvelope(env, s.identity.EncryptionPriv, wk)
}

// Directory returns all published public keys with their display names.
func (s *Service) Directory(ctx context.Context) ([]PublicKeyEntry, error) {
	return s.store.ListPublicKeys(ctx)
}

// ListItems returns the owner's own item envelopes.
func (s *Service) ListItems(ctx context.Context) ([]Envelope, error) {
	return s.store.ListItems(ctx)
}

// ListSharedLinks returns the user's durable records of items shared with them.
func (s *Service) ListSharedLinks(ctx context.Context) ([]SharedLink, error) {
	return s.store.ListSharedLinks(ctx)
}

// DeleteSharedLink removes a SharedLink, e.g. after discovering its shared
// envelope no longer exists (a missed revoke degrades to a dead link).
// Deleting an already-absent link is not an error.
func (s *Service) DeleteSharedLink(ctx context.Context, shareID string) error {
	if err := s.store.DeleteSharedLink(ctx, shareID); err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("deleting shared link %s: %w", shareID, err)
	}
	return nil
}

// ListShareRecords returns the owner's outgoing shares for itemID.
func (s *Service) ListShareRecords(ctx context.Context, itemID string) ([]ShareRecord, error) {
	all, err := s.store.ListShareRecords(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing share records: %w", err)
	}
	recs := make([]ShareRecord, 0, len(all))
	for _, rec := range all {
		if rec.ItemID == itemID {
			recs = append(recs, rec)
		}
	}
	return recs, nil
}

// UpdateItem re-encrypts content under the item's existing key (with a fresh
// nonce — never reuse a nonce under the same key) and writes the owned envelope
// back, then rewrites every shared envelope made from the item so recipients
// see the edit without re-sharing. Recipients' wrapped keys remain valid
// because the item key is unchanged.
func (s *Service) UpdateItem(ctx context.Context, itemID string, content items.Content) (Envelope, error) {
	env, err := s.store.GetItem(ctx, itemID)
	if err != nil {
		return Envelope{}, fmt.Errorf("getting item %s: %w", itemID, err)
	}
	ownerWK, ok := findOwnerKey(env, s.entityID)
	if !ok {
		return Envelope{}, fmt.Errorf("no wrapped key for owner in item %s", itemID)
	}
	itemKey, err := crypto.UnwrapKey(s.identity.EncryptionPriv, ownerWK.EphemeralPub, ownerWK.Nonce, ownerWK.Wrapped)
	if err != nil {
		return Envelope{}, fmt.Errorf("unwrapping item key: %w", err)
	}

	contentBytes, err := items.Encode(content)
	if err != nil {
		return Envelope{}, fmt.Errorf("encoding content: %w", err)
	}
	nonce, ciphertext, err := crypto.Seal(itemKey, contentBytes)
	if err != nil {
		return Envelope{}, fmt.Errorf("sealing content: %w", err)
	}

	env.Type = content.Kind()
	env.Nonce = nonce
	env.Ciphertext = ciphertext

	if err := s.store.PutItem(ctx, itemID, env); err != nil {
		return Envelope{}, fmt.Errorf("storing item %s: %w", itemID, err)
	}

	recs, err := s.ListShareRecords(ctx, itemID)
	if err != nil {
		return Envelope{}, err
	}
	for _, rec := range recs {
		sharedEnv := env
		sharedEnv.ID = rec.ShareID
		if _, err := s.store.PutSharedEnvelope(ctx, rec.ShareID, sharedEnv); err != nil {
			return Envelope{}, fmt.Errorf("updating shared envelope %s: %w", rec.ShareID, err)
		}
	}
	return env, nil
}

// DeleteItem permanently deletes an owned item. Every outstanding share of the
// item is revoked first: the shared envelope is deleted (the security action),
// a revoke message is sent so the recipient's client drops its link, and the
// owner's ShareRecord is removed. Share cleanup runs before the owned envelope
// is deleted and tolerates already-deleted records, so a partial failure is
// retryable.
func (s *Service) DeleteItem(ctx context.Context, itemID string) error {
	recs, err := s.ListShareRecords(ctx, itemID)
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if err := s.revokeShare(ctx, rec.ShareID, rec.RecipientID); err != nil {
			return err
		}
	}
	if err := s.store.DeleteItem(ctx, itemID); err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("deleting item %s: %w", itemID, err)
	}
	return nil
}

// Share shares itemID with recipientID.
//
// It decrypts the owner's item key, wraps it for the recipient, writes a shared
// envelope to the shared namespace (retaining the owner's wrapped key so the
// owner can still decrypt it), and drops a share message in the recipient's inbox.
func (s *Service) Share(ctx context.Context, itemID, recipientID string) error {
	env, err := s.store.GetItem(ctx, itemID)
	if err != nil {
		return fmt.Errorf("getting item %s: %w", itemID, err)
	}

	ownerWK, ok := findOwnerKey(env, s.entityID)
	if !ok {
		return fmt.Errorf("no wrapped key for owner in item %s", itemID)
	}
	itemKey, err := crypto.UnwrapKey(s.identity.EncryptionPriv, ownerWK.EphemeralPub, ownerWK.Nonce, ownerWK.Wrapped)
	if err != nil {
		return fmt.Errorf("unwrapping item key: %w", err)
	}

	recipientPub, err := s.store.GetPublicKey(ctx, recipientID)
	if err != nil {
		return fmt.Errorf("getting public key for %s: %w", recipientID, err)
	}
	recipientWK, err := WrapKeyForRecipient(itemKey, recipientID, recipientPub)
	if err != nil {
		return fmt.Errorf("wrapping key for recipient: %w", err)
	}
	recipientWKBytes, err := marshalWrappedKey(recipientWK)
	if err != nil {
		return err
	}

	// Write the shared envelope. It is a shallow copy of the original — same
	// ciphertext, owner's wrapped key in Recipients; recipient's key is NOT here.
	shareID := newID()
	sharedEnv := env
	sharedEnv.ID = shareID

	version, err := s.store.PutSharedEnvelope(ctx, shareID, sharedEnv)
	if err != nil {
		return fmt.Errorf("writing shared envelope: %w", err)
	}

	// Record the outgoing share before notifying the recipient, so that even
	// if the message send fails the owner can find and clean up the envelope.
	rec := ShareRecord{
		ShareID:     shareID,
		ItemID:      itemID,
		RecipientID: recipientID,
		ItemType:    string(env.Type),
	}
	if err := s.store.PutShareRecord(ctx, rec); err != nil {
		return fmt.Errorf("recording share %s: %w", shareID, err)
	}

	msg := Message{
		Type:       MessageShare,
		ShareID:    shareID,
		SenderID:   s.entityID,
		EnvVersion: version,
		Timestamp:  time.Now().UTC(),
		Share: &SharePayload{
			SharePath:  sharePath(s.entityID, shareID),
			WrappedKey: recipientWKBytes,
			ItemType:   string(env.Type),
			OwnerID:    s.entityID,
		},
	}
	if err := s.store.SendMessage(ctx, recipientID, newID(), msg); err != nil {
		return fmt.Errorf("sending share message to %s: %w", recipientID, err)
	}
	return nil
}

// Revoke removes a recipient's access to a shared item.
//
// It deletes the shared envelope (the real security action — the ciphertext is
// gone), drops a revoke message so the recipient's client can remove the stale
// SharedLink, and removes the owner's ShareRecord. recipientID is required to
// route the revoke message. Revoking an already-revoked share is not an error,
// so a partially failed revoke can be retried.
func (s *Service) Revoke(ctx context.Context, shareID, recipientID string) error {
	return s.revokeShare(ctx, shareID, recipientID)
}

func (s *Service) revokeShare(ctx context.Context, shareID, recipientID string) error {
	if err := s.store.DeleteSharedEnvelope(ctx, shareID); err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("deleting shared envelope %s: %w", shareID, err)
	}

	msg := Message{
		Type:      MessageRevoke,
		ShareID:   shareID,
		SenderID:  s.entityID,
		Timestamp: time.Now().UTC(),
	}
	if err := s.store.SendMessage(ctx, recipientID, newID(), msg); err != nil {
		return fmt.Errorf("sending revoke message to %s: %w", recipientID, err)
	}

	if err := s.store.DeleteShareRecord(ctx, shareID); err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("deleting share record %s: %w", shareID, err)
	}
	return nil
}

// ProcessInbox reads all pending inbox messages and applies them.
//
// For share messages: writes a SharedLink, then deletes the message.
// For revoke messages: removes the SharedLink, then deletes the message.
//
// Each handler writes/removes the link before deleting the message so that a
// crash between the two steps is self-healing — the next ProcessInbox call
// will re-process the still-present message.
func (s *Service) ProcessInbox(ctx context.Context) error {
	entries, err := s.store.ListInboxMessages(ctx)
	if err != nil {
		return fmt.Errorf("listing inbox: %w", err)
	}
	for _, entry := range entries {
		if err := s.processEntry(ctx, entry); err != nil {
			return err
		}
	}
	return nil
}

// OpenSharedItem fetches the shared envelope identified by link and decrypts it
// using the recipient's wrapped key stored in the link.
func (s *Service) OpenSharedItem(ctx context.Context, link SharedLink) (items.Content, error) {
	wk, err := unmarshalWrappedKey(link.WrappedKey)
	if err != nil {
		return nil, fmt.Errorf("parsing wrapped key in link %s: %w", link.ShareID, err)
	}

	ownerID, shareID, err := parseSharePath(link.SharePath)
	if err != nil {
		return nil, fmt.Errorf("parsing share path %q: %w", link.SharePath, err)
	}

	env, _, err := s.store.GetSharedEnvelope(ctx, ownerID, shareID)
	if err != nil {
		return nil, fmt.Errorf("getting shared envelope: %w", err)
	}

	return OpenEnvelope(env, s.identity.EncryptionPriv, wk)
}

func (s *Service) processEntry(ctx context.Context, entry InboxEntry) error {
	switch entry.Msg.Type {
	case MessageShare:
		return s.processShare(ctx, entry)
	case MessageRevoke:
		return s.processRevoke(ctx, entry)
	default:
		return fmt.Errorf("unknown message type %q (share %s)", entry.Msg.Type, entry.Msg.ShareID)
	}
}

func (s *Service) processShare(ctx context.Context, entry InboxEntry) error {
	if entry.Msg.Share == nil {
		return fmt.Errorf("share message %s missing payload", entry.Msg.ShareID)
	}

	// Idempotency: if we already have a link at this version or newer, skip writing
	// and just clean up the message.
	existing, err := s.store.GetSharedLink(ctx, entry.Msg.ShareID)
	if err == nil && existing.EnvVersion >= entry.Msg.EnvVersion {
		return s.store.DeleteInboxMessage(ctx, entry.ID)
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("checking existing link for share %s: %w", entry.Msg.ShareID, err)
	}

	link := SharedLink{
		ShareID:    entry.Msg.ShareID,
		SharePath:  entry.Msg.Share.SharePath,
		WrappedKey: entry.Msg.Share.WrappedKey,
		OwnerID:    entry.Msg.Share.OwnerID,
		ItemType:   entry.Msg.Share.ItemType,
		EnvVersion: entry.Msg.EnvVersion,
	}
	// Write link first — if we crash before deleting the message, the next
	// ProcessInbox call will re-enter and the idempotency check will skip the write.
	if err := s.store.PutSharedLink(ctx, link); err != nil {
		return fmt.Errorf("writing shared link for share %s: %w", entry.Msg.ShareID, err)
	}
	return s.store.DeleteInboxMessage(ctx, entry.ID)
}

func (s *Service) processRevoke(ctx context.Context, entry InboxEntry) error {
	// Remove link first — if we crash before deleting the message, the next
	// ProcessInbox call will re-enter; ErrNotFound on the link is swallowed.
	if err := s.store.DeleteSharedLink(ctx, entry.Msg.ShareID); err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("removing shared link for revoke %s: %w", entry.Msg.ShareID, err)
	}
	return s.store.DeleteInboxMessage(ctx, entry.ID)
}

func sharePath(ownerID, shareID string) string {
	return ownerID + "/" + shareID
}

func parseSharePath(path string) (ownerID, shareID string, err error) {
	i := strings.IndexByte(path, '/')
	if i < 0 {
		return "", "", fmt.Errorf("invalid share path %q: expected ownerID/shareID", path)
	}
	return path[:i], path[i+1:], nil
}