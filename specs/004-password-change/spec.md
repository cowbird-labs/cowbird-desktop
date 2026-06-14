# Feature Specification: Change Unlock Password

**Feature Branch**: `004-password-change`
**Status**: Draft
**Created**: 2026-06-14

## Summary

A signed-in user can change the password that unlocks their identity. The
unlock password protects the at-rest private key (it is separate from the Vault
auth credential), and today there is no way to change it short of losing access.
This feature adds a change-password flow: the user supplies their current
password and a new one, and the locked identity is re-wrapped under a key
derived from the new password. The underlying keypair does not change, so no
item contents are re-encrypted and no recipient's access is disturbed.

This is the cheap, frequent half of the password/key separation (001 research.md
Decision 12). Key rotation — generating a new keypair after suspected
compromise — is a distinct, heavier feature and is out of scope here.

## User Scenarios

### User Story 1 — Change the unlock password (Priority: P1)

A signed-in user opens a change-password form, enters their current password
and a new password (with confirmation), and submits. On success their data
remains accessible and the next unlock requires the new password.

**Why this priority**: This is the feature.

**Acceptance Scenarios**:

1. **Given** a signed-in user, **When** they enter their correct current
   password and a confirmed new password and submit, **Then** the change
   succeeds and all their items remain readable in the current session without
   reload.
2. **Given** a successful password change, **When** the user re-launches
   Cowbird and unlocks, **Then** the new password works and the old password is
   rejected.
3. **Given** a successful password change, **Then** no item contents are
   re-encrypted and items already shared with the user, or shared by them,
   remain accessible to all parties (the keypair is unchanged).

### User Story 2 — Wrong current password is rejected (Priority: P1)

The current password is verified before any change is written.

**Acceptance Scenarios**:

1. **Given** the change-password form, **When** the user enters an incorrect
   current password, **Then** the change is refused with a generic error and
   the stored identity is left untouched.
2. **Given** the form, **When** the new password and its confirmation do not
   match, **Then** submission is blocked before any Vault write.

### User Story 3 — Failure leaves a usable state (Priority: P2)

A failure while writing the re-wrapped identity must not lock the user out.

**Acceptance Scenarios**:

1. **Given** a change in progress, **When** the Vault write fails, **Then** the
   error is surfaced and the user can still unlock with their existing
   (unchanged) password — both now and after relaunch.

## Requirements

### Functional Requirements

- **FR-001**: A signed-in user MUST be able to change their unlock password by
  supplying the current password and a new password.
- **FR-002**: The current password MUST be verified (by decrypting the stored
  locked identity) before any change is written; an incorrect current password
  MUST yield a generic error and leave the stored identity unchanged.
- **FR-003**: The new password and its confirmation MUST match before any Vault
  write occurs.
- **FR-004**: Changing the password MUST re-wrap the existing key material under
  a key derived from the new password with a freshly generated Argon2id salt; it
  MUST NOT re-encrypt item contents or alter the user's keypair or published
  public key.
- **FR-005**: After a successful change the new password MUST unlock the
  identity on subsequent launches and the old password MUST NOT.
- **FR-006**: The current in-memory session MUST remain fully usable after a
  successful change without re-entering the password (the unlocked identity is
  unchanged).
- **FR-007**: A write failure MUST leave the previously stored locked identity
  intact and unlockable with the old password (single-write replacement; no
  destructive pre-step).
- **FR-008**: Failures MUST surface as user-readable errors.

### Key Entities

- **LockedIdentity** (existing, `internal/crypto`): the at-rest encrypted
  keypair stored at `users/<entityID>/identity`. The change operation replaces
  it with a new `LockedIdentity` (new salt, new nonce, new ciphertext) wrapping
  the same private key.

## Assumptions

- The user is already unlocked when changing their password (the flow lives in
  the main window, not the unlock window). Verifying the current password again
  is still required — it confirms intent and matches the at-rest data being
  re-wrapped, and guards a walk-up attacker on an unlocked session.
- A single KV v2 write to the identity path atomically supersedes the prior
  version; cowbird does not need the old version retained, so prior versions
  may remain in KV history per the mount's retention policy. Acceptable: old
  versions are still only decryptable with the old password.

## Out of Scope

- Key rotation (new keypair, re-wrap all item keys, re-publish public key):
  separate future feature.
- Password strength *policy* / enforcement (the existing strength meter from the
  unlock window is reused as advisory only).
- Recovering a forgotten current password (there is no recovery path by design;
  see 001 spec User Story 4 — key export/import is the recovery mechanism).
- Coordinating concurrent sessions (matters for key rotation, not for a
  password re-wrap that leaves the keypair untouched).
