package sharing

import (
	"time"

	"cowbird/internal/items"
)

// MessageType distinguishes share and revoke inbox messages.
type MessageType string

const (
	MessageShare  MessageType = "share"
	MessageRevoke MessageType = "revoke"
)

// WrappedKey holds an item key encrypted to a specific recipient's X25519 public key.
type WrappedKey struct {
	RecipientID  string `json:"recipient_id"`
	EphemeralPub []byte `json:"ephemeral_pub"`
	Nonce        []byte `json:"nonce"`
	Wrapped      []byte `json:"wrapped"` // item key encrypted to recipient
}

// Envelope is the at-rest encrypted form of an item.
// Ciphertext is the item content encrypted with the item key.
// Recipients holds the owner's wrapped copy of the item key. For shared items,
// the recipient's wrapped key is NOT stored here — it travels via the inbox.
type Envelope struct {
	ID      string         `json:"id"`
	Type    items.ItemType `json:"type"`
	OwnerID string         `json:"owner_id"`
	// Format is the content-AEAD format: 0 (absent) for legacy envelopes sealed
	// with no associated data, contentFormatAAD for content bound to the owner
	// and type. See contentAAD.
	Format     int          `json:"format,omitempty"`
	Recipients []WrappedKey `json:"recipients,omitempty"`
	Nonce      []byte       `json:"nonce"`
	Ciphertext []byte       `json:"ciphertext"`
	Signature  []byte       `json:"signature,omitempty"` // deferred
}

// Message is a consume-and-delete inbox message written by the sender.
// Signature is the sender's Ed25519 signature over the authenticated fields
// (see signingBytes); it is empty for legacy senders who predate signing keys.
type Message struct {
	Type       MessageType   `json:"type"`
	ShareID    string        `json:"share_id"`    // opaque UUID identifying the share
	SenderID   string        `json:"sender_id"`   // informational
	EnvVersion int64         `json:"env_version"` // KV v2 version; ordering tiebreaker
	Timestamp  time.Time     `json:"timestamp"`   // display only, not authoritative
	Share      *SharePayload `json:"share,omitempty"`
	Signature  []byte        `json:"signature,omitempty"`
}

// SharePayload carries the data a recipient needs to access a newly shared item.
// WrappedKey is a JSON-encoded inner WrappedKey struct (EphemeralPub+Nonce+Wrapped).
type SharePayload struct {
	SharePath  string `json:"share_path"`  // ownerID/shareID
	WrappedKey []byte `json:"wrapped_key"` // serialized WrappedKey for the recipient
	ItemType   string `json:"item_type"`   // for list display before decrypt
	OwnerID    string `json:"owner_id"`
}

// SharedLink is a durable record in the recipient's own subtree.
// It is the standing record of an item shared with them.
// WrappedKey is a JSON-encoded inner WrappedKey struct.
type SharedLink struct {
	ShareID    string `json:"share_id"`
	SharePath  string `json:"share_path"`
	WrappedKey []byte `json:"wrapped_key"` // serialized WrappedKey for this recipient
	OwnerID    string `json:"owner_id"`
	ItemType   string `json:"item_type"`
	EnvVersion int64  `json:"env_version"` // version of the envelope last acted on
}

// ShareRecord is the owner's durable record of one outgoing share, stored in
// the owner's own subtree. It is what lets item edits propagate to shared
// envelopes, lets DeleteItem revoke outstanding shares, and will back the
// share-management UI.
type ShareRecord struct {
	ShareID     string `json:"share_id"`
	ItemID      string `json:"item_id"` // the owned item this share was made from
	RecipientID string `json:"recipient_id"`
	ItemType    string `json:"item_type"`
}

// InboxEntry wraps a Message with its inbox path identifier.
// The ID is the Vault key name (not stored inside the message itself),
// needed to delete the message after processing.
type InboxEntry struct {
	ID  string
	Msg Message
}