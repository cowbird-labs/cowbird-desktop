# Feature Specification: Key Import (Restore from Recovery File)

**Feature Branch**: `007-key-import`
**Status**: Draft
**Created**: 2026-06-14

## Summary

A user restores access by importing the passphrase-protected recovery file they
exported (feature 006). Import lives in the **unlock window**, because recovery
is needed precisely when the user cannot sign in normally: they have forgotten
their unlock password, or the Vault-stored identity is missing (a fresh or
cleared Vault, a new deployment). Importing decrypts the recovery file,
re-locks the recovered keypair under a new unlock password the user chooses, and
writes it back as the Vault-stored identity — after which the user signs in
normally and regains access to their items and any items still shared with them
(the keypair is the original).

Because importing overwrites the Vault identity, and a wrong file would orphan
every item (they are wrapped to the real key), import compares the recovered
public key against the user's published public key and warns on a mismatch.
This completes 001 spec User Story 4 and is the sequel to 006.

## User Scenarios

### User Story 1 — Restore on a fresh/cleared Vault (Priority: P1)

A user whose Vault has no stored identity (new deployment, cleared record)
imports their recovery file instead of creating a brand-new keypair, preserving
access to their existing items.

**Why this priority**: Without this, a user with no Vault-stored identity can
only create a *new* keypair, which orphans all of their existing items.

**Acceptance Scenarios**:

1. **Given** an authenticated user whose Vault has no stored identity, **When**
   they choose to import, select their recovery file, enter its passphrase, and
   set a new unlock password, **Then** the recovered identity is stored and they
   enter the app with access to their own items.
2. **Given** no published public key exists yet, **When** they import, **Then**
   no mismatch warning is shown (there is nothing to conflict with).

### User Story 2 — Recover a forgotten unlock password (Priority: P1)

A returning user who has forgotten their unlock password imports their recovery
file to set a new one.

**Acceptance Scenarios**:

1. **Given** the unlock (enter-password) screen, **Then** an option to import a
   recovery file instead is available.
2. **Given** a user imports a recovery file whose key matches their published
   public key and sets a new unlock password, **Then** the Vault identity is
   replaced, the new password unlocks it, and their items remain readable.
3. **Given** a user imports a file whose key does **not** match their published
   public key, **Then** they are warned that the file belongs to a different
   identity and that existing items will become unreadable, and the import
   proceeds only on explicit confirmation.

### User Story 3 — Bad file or passphrase (Priority: P2)

Import validates before it changes anything.

**Acceptance Scenarios**:

1. **Given** an incorrect export passphrase or an unreadable/malformed file,
   **When** the user submits, **Then** import is refused with a clear error and
   nothing is written to Vault.
2. **Given** the new unlock password and its confirmation do not match (or are
   empty), **Then** import is blocked before any Vault write.

## Requirements

### Functional Requirements

- **FR-001**: The unlock window MUST offer an import option in both states
  (first-run / no stored identity, and returning-user / enter-password).
- **FR-002**: Import MUST decrypt the chosen recovery file with the
  user-supplied export passphrase using the existing `crypto.ImportKey`; a wrong
  passphrase or malformed file MUST be refused with a clear error and MUST write
  nothing.
- **FR-003**: Import MUST store the recovered identity as the Vault locked
  identity, re-locked under a new unlock password the user chooses and confirms;
  that password becomes the unlock password going forward.
- **FR-004**: Before overwriting an existing identity, import MUST compare the
  recovered public key with the user's published public key
  (`pubkeys/<entityID>`); on mismatch it MUST warn that the file is a different
  identity and existing items will be unreadable, and proceed only on explicit
  confirmation.
- **FR-005**: When no published public key exists, import MUST proceed without a
  mismatch prompt (restore into an empty/fresh Vault).
- **FR-006**: After import the user MUST be taken into the app with the restored
  identity; the published public key MUST be (re)published so the directory is
  current.
- **FR-007**: The recovered private key MUST be persisted only in locked form
  (under the new unlock password); it MUST NOT be written in plaintext.
- **FR-008**: Import MUST clear any in-progress key-rotation marker
  (`identity.prev`), since the imported identity is a fresh starting point and a
  stale marker (locked under an unknown old password) would otherwise block the
  next unlock.
- **FR-009**: Failures MUST surface as user-readable errors and import MUST be
  retryable.

### Key Entities

- **ExportedKey** (existing): the recovery file consumed by import.
- **LockedIdentity** (existing): written to Vault under the new unlock password.
- **Public-key directory entry** (existing): the integrity reference for the
  mismatch check (FR-004) and (re)published on success (FR-006).

## Assumptions

- The user has already configured Vault and authenticated; import runs in the
  unlock window, after setup/auth, so `Vault` (and the entity ID) is available.
- The published public key is a sufficient reference for catching an *accidental*
  wrong file. Under the soft trust model (001 Decision 3) the operator could
  alter it, so this is a usability safeguard, not a security guarantee.
- Importing intentionally overwrites the Vault identity; any prior locked
  identity (e.g. under a forgotten password) is discarded — that is the point of
  recovery.
- The recovery file contains the user's encryption keypair only; Ed25519 signing
  keys are deferred everywhere.

## Out of Scope

- Importing from inside the app (the main window); recovery is an unlock-time
  action.
- Merging or holding multiple identities/keypairs.
- Re-keying or rotation as part of import (the user can rotate afterward if they
  believe the old key was exposed).
- Changing Vault connection/auth settings (that is the setup window's job).
