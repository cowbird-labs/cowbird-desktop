package sharing

import (
	"context"
	"errors"
)

// ErrNotFound is returned by Store methods when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// PublicKeyEntry is one entry in the public-key directory. Name is the
// advisory display name published with the key; it may be empty (records
// published before names existed) and is not authenticated — under the soft
// trust model the operator could alter it, as they could the key itself.
type PublicKeyEntry struct {
	EntityID string
	Pub      [32]byte
	Name     string
}

// Store abstracts the storage backend for all sharing operations.
// The Vault implementation lives in the vault package.
//
// Methods that read only from the current user's subtree (e.g. ListItems,
// ListInboxMessages, ListSharedLinks) are implicitly scoped to the authenticated
// entity; the entity ID is encoded in the Vault token, not passed explicitly.
type Store interface {
	// Own items (users/<entityID>/items/<itemID>)
	PutItem(ctx context.Context, itemID string, env Envelope) error
	GetItem(ctx context.Context, itemID string) (Envelope, error)
	DeleteItem(ctx context.Context, itemID string) error
	ListItems(ctx context.Context) ([]Envelope, error)

	// Public-key directory (pubkeys/<entityID>; read-all, write-own)
	GetPublicKey(ctx context.Context, entityID string) ([32]byte, error)
	PutPublicKey(ctx context.Context, entityID string, pub [32]byte, name string) error
	ListPublicKeys(ctx context.Context) ([]PublicKeyEntry, error)

	// Shared envelopes (shared/<ownerEntityID>/<shareID>; written by owner, readable by all)
	// PutSharedEnvelope returns the storage version for use as an ordering tiebreaker
	// in inbox messages.
	PutSharedEnvelope(ctx context.Context, shareID string, env Envelope) (version int64, err error)
	GetSharedEnvelope(ctx context.Context, ownerID, shareID string) (env Envelope, version int64, err error)
	DeleteSharedEnvelope(ctx context.Context, shareID string) error

	// Inbox (inbox/<recipientEntityID>/<msgID>; create-only for senders, read+delete for self)
	SendMessage(ctx context.Context, recipientID, msgID string, msg Message) error
	ListInboxMessages(ctx context.Context) ([]InboxEntry, error)
	DeleteInboxMessage(ctx context.Context, msgID string) error

	// SharedLinks (users/<entityID>/links/<shareID>; own subtree, durable state)
	PutSharedLink(ctx context.Context, link SharedLink) error
	GetSharedLink(ctx context.Context, shareID string) (SharedLink, error)
	DeleteSharedLink(ctx context.Context, shareID string) error
	ListSharedLinks(ctx context.Context) ([]SharedLink, error)

	// ShareRecords (users/<entityID>/shares/<shareID>; owner's outgoing shares)
	PutShareRecord(ctx context.Context, rec ShareRecord) error
	ListShareRecords(ctx context.Context) ([]ShareRecord, error)
	DeleteShareRecord(ctx context.Context, shareID string) error
}