# Feature Specification: Key Rotation

**Feature Branch**: `005-key-rotation`
**Status**: Draft
**Created**: 2026-06-14

## Summary

A user who suspects their private key is compromised rotates it: cowbird
generates a fresh X25519 keypair, re-encrypts every owned item under a new item
key wrapped to the new public key, re-distributes the new item keys to existing
recipients, publishes the new public key, and discards the old keypair. Because
each item's content is re-encrypted under a brand-new item key, any item key an
attacker may have harvested with the old private key is invalidated — this is
the real compromise-recovery guarantee that distinguishes rotation from the
cheap password change (004), which only re-wraps the same keypair.

Rotation touches a lot of stored data and must survive interruption, so it is
designed to be **idempotent and resumable**: the old key is retained in a
transitional slot until every owned item is migrated, and an interrupted
rotation completes on the next unlock. This implements 001 spec User Story 5
scenario 2 and 001 research.md Decision 12.

## User Scenarios

### User Story 1 — Rotate after suspected compromise (Priority: P1)

A signed-in user opens the rotate-key action, confirms with their unlock
password, and acknowledges the warning. Cowbird re-secures all of their items
under a new keypair and publishes the new public key.

**Why this priority**: This is the feature.

**Acceptance Scenarios**:

1. **Given** a signed-in user with several owned items, **When** they rotate
   their key, **Then** every owned item is re-encrypted under a new item key
   wrapped to the new public key, and all remain readable in the session
   afterward.
2. **Given** rotation completed, **When** anyone reads the prior (old-key)
   ciphertext or a harvested old item key, **Then** it no longer decrypts the
   current content (content was re-encrypted under fresh item keys).
3. **Given** rotation completed, **When** the user relaunches and unlocks,
   **Then** the new password-wrapped keypair unlocks normally and the old
   keypair is no longer stored.
4. **Given** rotation completed, **Then** the user's published public key is the
   new one, so future shares from other users wrap to it.

### User Story 2 — Recipients keep access without re-sharing (Priority: P1)

Items the user has shared with others remain readable to those recipients after
rotation, with no action required from the recipients.

**Acceptance Scenarios**:

1. **Given** Paul shared an item with Aubry and then rotates his key, **When**
   Aubry's client next processes its inbox, **Then** she has the new item key
   and can still read the item.
2. **Given** rotation re-distributes shares, **When** a recipient's current
   published public key is used, **Then** the re-wrap targets that current key
   (not a stale one).

### User Story 3 — Interrupted rotation self-heals (Priority: P1)

A rotation interrupted by a crash, network failure, or quit does not strand the
user's data and finishes on the next unlock.

**Acceptance Scenarios**:

1. **Given** a rotation that fails partway (some items migrated, some not),
   **When** the user retries in the same session, **Then** rotation resumes and
   completes without re-doing already-migrated items destructively.
2. **Given** a rotation interrupted before completion, **When** the user
   relaunches and unlocks, **Then** the client detects the in-progress rotation
   and completes it before presenting the item list.
3. **Given** an interrupted rotation, **Then** at no point is any owned item
   left unreadable by both the old and the new key — every owned item is always
   readable by whichever key currently wraps it.

### User Story 4 — Items shared *with* the user need owner re-share (Priority: P2)

Rotation cannot re-secure items other users shared with the rotating user; only
those owners can. The UI is honest about this.

**Acceptance Scenarios**:

1. **Given** Aubry has items shared *with* her and she rotates her key, **When**
   she views those items afterward, **Then** they appear as unreadable (not as
   an error/crash) until their owners re-share to her new key.
2. **Given** an owner re-shares to Aubry's new key, **When** Aubry processes her
   inbox, **Then** the item becomes readable again.

## Requirements

### Functional Requirements

- **FR-001**: A signed-in user MUST be able to initiate key rotation from the
  main window, gated by confirming the current unlock password and an explicit
  warning that this is for compromise recovery and that other open sessions
  should be closed first.
- **FR-002**: Rotation MUST generate a new X25519 keypair, store the new private
  key locked under the unlock password, and publish the new public key to the
  directory.
- **FR-003**: Every owned item MUST be re-encrypted under a freshly generated
  item key wrapped to the new public key; the old item key and old-key wrapping
  MUST NOT be able to decrypt the rotated content.
- **FR-004**: For every outstanding outgoing share, the new item key MUST be
  re-wrapped to the recipient's *current* published public key, the shared
  envelope rewritten, and the recipient notified through the existing inbox
  protocol, so recipients regain access without action of their own.
- **FR-005**: Rotation MUST be idempotent and resumable. The old keypair MUST
  remain recoverable (stored, password-locked, in a transitional slot) until
  every owned item has been migrated; an interrupted rotation MUST complete on
  the next unlock before the item list is shown.
- **FR-006**: At no point during rotation MUST an owned item be unreadable by
  both keys; each owned item MUST always be readable by whichever key currently
  wraps it.
- **FR-007**: Once every owned item is migrated and shares re-distributed, the
  transitional old keypair MUST be deleted, so a compromised old private key
  grants no further access to migrated content.
- **FR-008**: Items shared *with* the user (owned by others, wrapped to the
  user's old key) MUST degrade to unreadable rows after rotation, not errors,
  and the UI MUST indicate the owner must re-share. (Reuses existing
  unreadable-row handling.)
- **FR-009**: All failures MUST surface as user-readable errors and rotation
  MUST be retryable.
- **FR-010**: Item IDs and share IDs MUST be preserved across rotation (only key
  material and ciphertext change), so recipients' SharedLinks and the owner's
  ShareRecords continue to refer to the same shares.

### Key Entities

- **LockedIdentity** (existing): the canonical at-rest keypair at
  `users/<entityID>/identity`. Rotation replaces it with the new keypair.
- **Transitional prior identity** (new at-rest location):
  `users/<entityID>/identity.prev` — the old `LockedIdentity`, written at the
  start of rotation and deleted at completion. Its presence is the
  rotation-in-progress signal and the resume source for the old key.
- **Envelope** (existing): each owned item and each owned shared-envelope copy.
  Rotation rewrites `Nonce`, `Ciphertext`, and `Recipients[0]` (owner's wrapped
  key); `ID`, `Type`, `OwnerID` are preserved.
- **Public-key directory entry** (existing): republished with the new key.

## Assumptions

- A single active session is expected during rotation. Cowbird does not actively
  lock out concurrent sessions; a stale session editing under the old key during
  rotation is a known corruption risk (001 research.md Decision 12), mitigated
  only by the pre-rotation warning to close other sessions.
- Recipients' current public keys are read from the directory at rotation time;
  a recipient who has themselves rotated receives the re-wrap to their newest
  key.
- The unlock password is unchanged by rotation; the new keypair is locked under
  the same password the user confirms. (To also change the password, use 004
  separately.)
- The KV v2 mount may retain prior versions of rotated envelopes in history;
  those old versions remain decryptable only with the old key, which is
  destroyed at completion. If history retention of pre-rotation ciphertext is a
  concern for a given deployment, version destruction is a follow-up (see Open
  Items in plan.md).

## Out of Scope

- Re-securing items shared *with* the user (only their owners can; US4 documents
  the degradation).
- Active multi-session coordination / distributed locking.
- Changing the unlock password as part of rotation (use 004).
- Destroying prior KV v2 versions of rotated envelopes (follow-up).
- Rotating the optional Ed25519 signing key (signing is deferred entirely).
