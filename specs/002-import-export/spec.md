# Feature Specification: Item Import and Export

**Feature Branch**: `002-import-export`
**Status**: Draft
**Created**: 2026-06-21

## Summary

Cowbird users need to get their item data into and out of the application in
bulk: to migrate from another tool, to keep an offline backup, or to move
between deployments. This feature adds a plaintext, full-fidelity export of the
user's own items to a file, and an import that re-creates items from such a
file. It is distinct from key recovery (`ExportIdentity` / `ImportIdentity`),
which moves *key material*, not item contents.

## User Scenarios

### User Story 1 — Export owned items to a file (Priority: P1)

A signed-in user exports all of the items they own to a single file they choose
on disk, so they have a portable backup or migration source.

**Why this priority**: Export is the foundation; import consumes its format, and
backup is the most-requested capability after basic storage.

**Acceptance Scenarios**:

1. **Given** a signed-in user with several items of different types, **When**
   they export, **Then** they receive one file containing every item they own,
   with all standard and custom fields intact.
2. **Given** the export is about to write secrets in the clear, **When** the user
   initiates it, **Then** the UI warns prominently that the file is unencrypted
   before any file is written.
3. **Given** an item the user cannot decrypt (e.g. shared-in, or an unreadable
   row), **When** they export, **Then** only items the user owns and can decrypt
   are included; unreadable items are skipped, not aborted.

### User Story 2 — Import items from a file (Priority: P1)

A user imports items from a cowbird export file; the items appear in their list,
encrypted to their own key.

**Acceptance Scenarios**:

1. **Given** a cowbird export file, **When** the user imports it, **Then** each
   item in the file is created as a new owned item and appears in their list on
   refresh.
2. **Given** a file that is malformed or not a cowbird export, **When** the user
   imports it, **Then** the import is rejected with a clear message and nothing
   is written.
3. **Given** a file where some entries decode and some do not, **When** the user
   imports it, **Then** the valid entries are imported and the user is told how
   many succeeded and how many were skipped.

## Requirements

### Functional Requirements

- **FR-001**: System MUST export all items the signed-in user owns and can
  decrypt to a single file, preserving item type and all standard and custom
  fields.
- **FR-002**: The export format MUST be cowbird-native JSON, self-describing
  (format tag + version) and losslessly re-importable by cowbird.
- **FR-003**: Export MUST include only items the user owns; items merely shared
  *with* the user are out of scope for v1.
- **FR-004**: System MUST warn the user, before writing, that an export file
  contains secrets in clear text and is not protected by a passphrase.
- **FR-005**: System MUST import items from a cowbird export file, creating each
  as a new owned item encrypted to the importing user's key.
- **FR-006**: Import MUST validate the file's format tag and version and reject
  anything it does not recognise without writing partial state.
- **FR-007**: Import MUST be resilient to individual bad entries: valid entries
  are imported and the user is told the success/skip counts.
- **FR-008**: Export and import core logic MUST live below the UI so a future CLI
  can reuse it (consistent with the project's UI/core decoupling).

### Key Entities

- **Export file**: A JSON document with a format tag, a schema version, an
  export timestamp, and an ordered list of item entries.
- **Item entry**: One item's type tag plus its decrypted content (the same
  `{type, data}` envelope `items.Encode` already produces).

## Assumptions

- Plaintext export is acceptable and expected for a v1 (matches Bitwarden /
  1Password unencrypted-export norms), provided the UI is honest that the file is
  unprotected. Passphrase-encrypted export reusing the `crypto.ExportKey`
  machinery is a natural follow-up, deferred here.
- Importing the same file twice creates duplicate items; de-duplication is not
  attempted in v1.
- Import does not preserve original item IDs; each imported item gets a fresh ID
  and is owned by the importing user.

## Out of Scope

- Importing from third-party formats (1Password / Bitwarden / Proton, CSV). The
  native JSON format is the only one in v1; foreign-format adapters can map onto
  it later.
- Exporting items shared *with* the user, or re-creating share relationships.
- Passphrase-encrypted export files (deferred; see Assumptions).
- Merge / de-duplication on import.
- CLI surface (core logic is structured to allow it; no command in v1).
