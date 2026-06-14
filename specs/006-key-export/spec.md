# Feature Specification: Key Export (Recovery File)

**Feature Branch**: `006-key-export`
**Status**: Draft
**Created**: 2026-06-14

## Summary

A signed-in user exports their private key to a passphrase-protected recovery
file and saves it somewhere safe and offline. This is cowbird's only recovery
mechanism: there is no operator-side reset (001 research.md Decision 11), so a
user who loses access to both their device and their Vault-stored identity has
no way back without this file. The crypto already exists (`crypto.ExportKey`);
this feature is the UI and the orchestration that gates it.

Scope is **export only**. Import (restoring from a recovery file on a new
device) is a separate flow with its own design questions — on a normal new
device the identity is already in Vault and the user simply unlocks, so import
matters only for the narrower cases (forgotten unlock password with a kept
recovery file, a cleared/corrupted Vault identity, or a fresh Vault). It is
deferred to a follow-up feature.

## User Scenarios

### User Story 1 — Export a recovery file (Priority: P1)

A signed-in user chooses to export their key, authorizes with their unlock
password, sets a passphrase to protect the file, picks a location, and receives
a passphrase-protected recovery file.

**Why this priority**: This is the feature, and it is the only recovery path —
until it exists, device loss is unrecoverable.

**Acceptance Scenarios**:

1. **Given** a signed-in user, **When** they open the export action, authorize
   with their correct unlock password, set and confirm an export passphrase, and
   choose a save location, **Then** a passphrase-protected recovery file is
   written there.
2. **Given** an exported recovery file, **When** anyone opens it without the
   export passphrase, **Then** its contents are unreadable (the private key is
   encrypted under a key derived from the passphrase).
3. **Given** the export dialog, **Then** the user is clearly told this file is
   the only way to recover access and must be stored safely and offline.

### User Story 2 — Authorization and validation (Priority: P1)

Export is gated and validated so it cannot be produced casually or with a
mistyped passphrase.

**Acceptance Scenarios**:

1. **Given** the export dialog, **When** the unlock password is incorrect,
   **Then** export is refused with a generic error and no file is written.
2. **Given** the export dialog, **When** the export passphrase and its
   confirmation do not match (or are empty), **Then** export is blocked before a
   file is written.
3. **Given** a chosen save location that cannot be written, **Then** the failure
   is surfaced and no misleading success is shown.

## Requirements

### Functional Requirements

- **FR-001**: A signed-in user MUST be able to export their private key from the
  main window to a file.
- **FR-002**: Export MUST be authorized by re-entering the current unlock
  password; an incorrect password MUST refuse export with a generic error and
  write nothing.
- **FR-003**: The recovery file MUST be encrypted under a separate,
  user-chosen export passphrase (independent of the unlock password), confirmed
  by a second entry; empty or mismatched passphrases MUST block export.
- **FR-004**: The recovery file MUST use the existing `crypto.ExportKey`
  format (`ExportedKey{Version, Salt, Nonce, Ciphertext}`), so a later import
  feature can restore it.
- **FR-005**: The user MUST choose the save location via the platform file
  dialog; the operation MUST surface write failures and MUST NOT report success
  unless the file was written.
- **FR-006**: The UI MUST state clearly that the recovery file is the only
  recovery mechanism (no operator reset) and should be stored safely offline.
- **FR-007**: The exported private key MUST NOT be written anywhere in plaintext
  (only the passphrase-encrypted form is persisted).

### Key Entities

- **ExportedKey** (existing, `internal/crypto`): the passphrase-protected file
  format. Reused unchanged.
- **Identity** (existing): the in-session keypair being exported.

## Assumptions

- The user is already unlocked; the unlock-password re-entry is a deliberate
  authorization gate against a walk-up attacker exfiltrating the key from an
  open session (matching Bitwarden/1Password, and consistent with cowbird's
  change-password and key-rotation flows, which also re-verify).
- The export passphrase is independent of the unlock password so the file's
  protection does not change when the unlock password changes, and so a user can
  use a stronger/written-down passphrase for an offline artifact.
- Writing the file to a user-chosen location hands control of the artifact to
  the user; cowbird does not retain, upload, or track it.

## Out of Scope

- **Import / restore** from a recovery file (separate follow-up; see Summary).
- Cloud/remote backup of the recovery file, or any cowbird-managed storage of it.
- Printed / QR / paper key formats.
- Re-export triggered automatically on key rotation (the user re-exports
  manually if they want a fresh file after rotating).
- Exporting anything other than the encryption keypair (Ed25519 signing keys are
  deferred entirely).
