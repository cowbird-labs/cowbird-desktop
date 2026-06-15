package sharing

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
)

// signingBytes produces the canonical, deterministic byte string that a share or
// revoke message's signature covers. Fields are length-prefixed so distinct
// field sets cannot collide, and a domain-separation prefix guards against the
// signature being valid in another context. The Signature field itself and the
// purely advisory fields (SenderID, Timestamp) are deliberately excluded: the
// signer is identified out of band (the share path's owner for shares, the
// link's owner for revokes), not by a self-asserted SenderID.
func signingBytes(msg Message) []byte {
	var b bytes.Buffer
	b.WriteString("cowbird-msg-v1\x00")
	writeField(&b, []byte(msg.Type))
	writeField(&b, []byte(msg.ShareID))
	var ver [8]byte
	binary.BigEndian.PutUint64(ver[:], uint64(msg.EnvVersion))
	b.Write(ver[:])
	if msg.Share != nil {
		writeField(&b, []byte(msg.Share.SharePath))
		writeField(&b, []byte(msg.Share.OwnerID))
		writeField(&b, []byte(msg.Share.ItemType))
		h := sha256.Sum256(msg.Share.WrappedKey)
		b.Write(h[:])
	}
	return b.Bytes()
}

func writeField(b *bytes.Buffer, p []byte) {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(p)))
	b.Write(n[:])
	b.Write(p)
}

// signMessage attaches the sender's signature to msg. If this identity has no
// signing key (a legacy identity not yet migrated), the message is left unsigned
// — recipients accept such messages only via the legacy fallback in
// verifyMessage.
func (s *Service) signMessage(msg *Message) {
	if len(s.identity.SigningPriv) != ed25519.PrivateKeySize {
		return
	}
	msg.Signature = ed25519.Sign(s.identity.SigningPriv, signingBytes(*msg))
}

// verifyMessage checks that msg is authentic as coming from claimedSigner.
//
//   - ok: the message may be trusted (a valid signature, or — see legacy — an
//     acceptable unsigned message during migration).
//   - legacy: claimedSigner has not published a signing key yet, so the message
//     could not be cryptographically verified; the caller falls back to whatever
//     other authority it has (the share path for shares).
//   - err: an infrastructure failure (not a verdict); the caller should retry
//     rather than discard, so a transient outage cannot drop a real message.
func (s *Service) verifyMessage(ctx context.Context, claimedSigner string, msg Message) (ok, legacy bool, err error) {
	sigPub, err := s.store.GetSigningKey(ctx, claimedSigner)
	if errors.Is(err, ErrNotFound) || len(sigPub) == 0 {
		return false, true, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("getting signing key for %s: %w", claimedSigner, err)
	}
	if len(msg.Signature) == 0 {
		// The signer has a published key but the message is unsigned — a downgrade;
		// reject it (definitively, not legacy).
		return false, false, nil
	}
	return ed25519.Verify(sigPub, signingBytes(msg), msg.Signature), false, nil
}
