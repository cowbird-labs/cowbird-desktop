# Feature Specification: Sharing Hardening

**Feature Branch**: `008-sharing-hardening`
**Status**: Draft
**Created**: 2026-06-15

## Summary

A security review of the sharing subsystem (001) found that the protocol layer
trusts data it should authenticate, and that revocation is enforced by obscurity
rather than cryptography. This feature closes those gaps so that cowbird's actual
behaviour matches the guarantees the 001 spec already claims (FR-004 through
FR-006).

Four root problems, in priority order:

1. **Share messages are unauthenticated.** Any user can `create` a message in
   any other user's inbox (policy `inbox/+/*`), and the recipient's client builds
   a durable `SharedLink` from message fields without verifying them. The
   displayed owner is taken from the attacker-controlled `Share.OwnerID` payload,
   not from the ACL-authenticated storage path. A user can therefore inject an
   item that appears to be "shared by" anyone — a phishing primitive. This is
   *not* covered by the 001 trust model, which only accepts operator key
   substitution, not user-to-user message forgery.
2. **Revocation does not re-key.** `Revoke` deletes the revoked recipient's
   envelope copy but leaves the item key unchanged, and edits reuse that key. A
   revoked recipient who retained the item key can still decrypt future edits via
   any other recipient's copy in the world-readable `shared/*` namespace. Today
   this is only blocked by opaque-UUID share IDs and the absence of `list` on
   others' subtrees — defence by obscurity, which contradicts the project's own
   "key names are not secret" stance.
3. **Envelope metadata is unauthenticated.** Content AEAD uses no associated
   data, so `Envelope.{Type, OwnerID, ID}` are malleable and a `WrappedKey` is
   not bound to the envelope it unlocks.
4. **The inbox is an unbounded, world-writable channel.** No size or rate
   limits; `ProcessInbox` handles everything synchronously at startup, so a
   flooded inbox stalls a victim's launch.

This work implements the authentication slot reserved in 001
(`Envelope.Signature`, the deferred Ed25519 signing key) and the re-key-on-
membership-change behaviour that 001 research.md Decision 6 assumed but did not
deliver.

## User Scenarios

### User Story 1 — A forged share cannot impersonate another user (Priority: P1)

A recipient can trust that an item attributed to a given sender was actually
shared by that sender.

**Why this priority**: Impersonated shares are a phishing vector against exactly
the trust relationship sharing is meant to establish; this is the most damaging
finding.

**Acceptance Scenarios**:

1. **Given** Mallory writes an envelope in her own shared namespace and injects
   an inbox message to Paul claiming `OwnerID` = Aubry's entity ID, **When**
   Paul's client processes the inbox, **Then** the message is rejected (the
   storage path's owner prefix does not match the claimed owner) and no
   SharedLink is created.
2. **Given** a genuine share from Aubry to Paul, **When** Paul's client
   processes it, **Then** the share's signature verifies against Aubry's
   published signing key and the item appears, correctly attributed to Aubry.
3. **Given** a share message whose signature does not verify against the named
   sender's published signing key, **When** the recipient processes it, **Then**
   it is rejected and surfaced as a discarded/untrusted message, not shown as a
   readable item.

### User Story 2 — Revocation cryptographically ends access (Priority: P1)

After an owner revokes a recipient, that recipient cannot read subsequent edits,
even if they kept the previous item key.

**Acceptance Scenarios**:

1. **Given** Paul shared an item with Aubry and Carol and then revokes Aubry,
   **When** Paul later edits the item, **Then** the edit is encrypted under a new
   item key wrapped only to Carol (and Paul), and Aubry's retained old item key
   does not decrypt any copy of the new content.
2. **Given** a revoke completes, **When** the previously shared ciphertext and
   all its prior versions are examined, **Then** the revoked recipient's copy and
   its version history are destroyed, not merely superseded.
3. **Given** revocation re-keys the item, **When** the remaining recipients next
   process their inboxes, **Then** they receive the new item key and retain
   access with no action of their own.

### User Story 3 — Tampered envelope metadata is detected (Priority: P2)

A modified `Type`, `OwnerID`, or `ID` on a stored envelope causes decryption to
fail rather than silently mislabel an item.

**Acceptance Scenarios**:

1. **Given** a stored envelope, **When** its `Type` or `OwnerID` is altered at
   rest, **Then** opening it fails authentication instead of decoding under the
   wrong type.

### User Story 4 — Inbox abuse cannot deny startup (Priority: P3)

A user flooded with inbox messages can still launch and use the app.

**Acceptance Scenarios**:

1. **Given** an inbox containing a very large number of messages, **When** the
   user launches, **Then** the main window becomes usable without waiting for the
   entire inbox to drain, and oversized messages are rejected.

## Requirements

### Functional Requirements

- **FR-001**: The recipient MUST derive a shared item's owner from the
  ACL-authenticated storage path (the `shared/<ownerEntityID>/...` prefix), not
  from any self-asserted payload field, and MUST reject a message whose claimed
  owner disagrees with that prefix.
- **FR-002**: Each user MUST publish an Ed25519 signing public key alongside
  their X25519 encryption key in the directory.
- **FR-003**: Share and revoke messages MUST be signed by the sender; recipients
  MUST verify the signature against the sender's published signing key and
  discard messages that do not verify.
- **FR-004**: Revoking a recipient (and deleting an item, which revokes all
  recipients) MUST re-key the item: re-encrypt content under a fresh item key and
  re-wrap to the remaining recipients, so a retained old item key cannot decrypt
  later edits.
- **FR-005**: Revocation MUST destroy the revoked envelope copy and its KV
  version history, not just overwrite or soft-delete it.
- **FR-006**: Item content encryption MUST bind the envelope's stable
  identifying metadata as authenticated associated data. Implemented as binding
  `OwnerID` and `Type`. `ID` is intentionally NOT bound: a shared copy reuses the
  owner's ciphertext under a different ID (the shareID), so binding ID would make
  shared copies undecryptable. ID-substitution is already prevented by each item
  having a unique item key.
- **FR-007**: The system MUST bound inbox processing so that an attacker cannot
  prevent a victim's client from starting: reject oversized messages and avoid
  blocking the UI on full inbox drain.
- **FR-008**: Changes MUST be backward-compatible with existing stored data, or
  ship a defined migration: identities published before signing keys existed,
  and envelopes encrypted without associated data, must continue to work or be
  migrated on next unlock.

### Key Entities

- **Signing key**: A per-user Ed25519 keypair, private part stored in the locked
  identity, public part published to the directory. Establishes share
  authenticity. (The slot reserved in 001 crypto `Identity`.)
- **Signed message**: An inbox share/revoke message plus a signature over its
  authenticated fields.
- **Authenticated envelope**: An item envelope whose content AEAD covers its
  identifying metadata.

## Assumptions

- The set of users is known and finite, but individual users are NOT all
  trusted: an insider may attempt to forge shares to others. (This tightens the
  001 model, which only addressed the operator.)
- The operator remains semi-trusted per 001 Decision 3; this feature does not
  attempt to defend against an operator forging the published signing-key
  directory (same accepted risk as the encryption-key directory).

## Out of Scope

- Out-of-band / TOFU verification of signing keys (same deferral as 001
  Decision 5; an operator who forges the directory is still out of scope).
- Forward secrecy for already-exposed data: rotation and revocation re-key
  future content but cannot un-expose what a recipient already read.
- Per-user inbox quotas enforced server-side (Vault policy cannot easily express
  them); FR-007 is a client-side robustness measure.

## Migration & Compatibility Notes

- **Signing keys** (FR-002): users created before this feature have no signing
  key. On next unlock, generate one, add it to the locked identity, and publish
  it. Until a sender has published a signing key, recipients fall back to the
  FR-001 path-authority check alone (reject on owner mismatch) and surface such
  shares as unverified.
- **Associated data** (FR-006): existing envelopes were sealed with no AAD.
  Decrypt MUST accept the no-AAD form for legacy items and re-seal with AAD on
  next write (e.g. on edit or rotation), or a one-time migration re-seals owned
  items. Define which on the plan side.
