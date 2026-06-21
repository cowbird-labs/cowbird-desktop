# Feature Specification: Third-Party Import/Export Formats

**Feature Branch**: `003-third-party-formats`
**Status**: Draft
**Created**: 2026-06-21

## Summary

Extend cowbird's bulk import/export (see `specs/002-import-export`) to interoperate
with four major password managers, in both directions:

| Manager       | Format            |
|---------------|-------------------|
| 1Password     | `.1pux` (ZIP+JSON)|
| Bitwarden     | JSON              |
| Proton Pass   | JSON              |
| LastPass      | CSV               |

A user can export their cowbird items into any of these formats (to migrate
away, or to seed another tool) and import a file produced by any of them (to
migrate in). The cowbird-native JSON format from feature 002 remains the
default and full-fidelity option.

## User Scenarios

### User Story 1 — Import from another manager (Priority: P1)

A user leaving 1Password / Bitwarden / Proton Pass / LastPass imports the file
that tool produced; their logins, cards, notes, and identities appear in
cowbird with as many fields preserved as the source format carries.

**Acceptance Scenarios**:

1. **Given** a Bitwarden JSON export, **When** the user imports it as Bitwarden,
   **Then** logins, cards, identities, and secure notes are created with their
   standard fields and any custom fields mapped onto cowbird custom fields.
2. **Given** a file whose contents do not match the chosen source format,
   **When** the user imports it, **Then** it is rejected with a clear message and
   nothing is written.
3. **Given** an export containing an item type cowbird has no exact match for,
   **When** it is imported, **Then** it is mapped to the closest cowbird type
   (falling back to a custom item) rather than dropped silently, or counted as a
   skip if it carries no usable data.

### User Story 2 — Export to another manager (Priority: P1)

A user exports their cowbird items in a chosen manager's format and successfully
imports the resulting file into that manager.

**Acceptance Scenarios**:

1. **Given** cowbird items of several types, **When** the user exports as
   Bitwarden JSON / Proton JSON / 1Password .1pux / LastPass CSV, **Then** the
   produced file is accepted by that manager's own importer.
2. **Given** the export writes secrets in clear text, **When** the user starts
   it, **Then** the same unencrypted-file warning as native export is shown
   first. (1Password `.1pux` is an unencrypted ZIP; it is not passphrase
   protected.)

### User Story 3 — Round-trip fidelity is honest (Priority: P2)

A user who exports to a foreign format and re-imports it understands what was
preserved and what was lossy.

**Acceptance Scenarios**:

1. **Given** a cowbird item with fields a target format cannot represent, **When**
   it is exported and re-imported, **Then** representable fields survive and
   non-representable ones are carried as notes/custom fields where the format
   allows, never silently corrupting other fields.

## Requirements

### Functional Requirements

- **FR-001**: System MUST import items from 1Password `.1pux`, Bitwarden JSON,
  Proton Pass JSON, and LastPass CSV files, in addition to cowbird-native JSON.
- **FR-002**: System MUST export items to each of those four formats, in addition
  to cowbird-native JSON.
- **FR-003**: Each adapter MUST map between the foreign schema and cowbird's item
  types (login, card, note, identity, password, custom) on a best-effort
  semantic basis: standard fields map to standard fields; fields with no native
  target map to cowbird custom fields (or, for export, the format's custom-field
  / notes facility) rather than being dropped.
- **FR-004**: Import MUST validate that the supplied bytes match the chosen
  source format and reject mismatches without writing partial state (consistent
  with feature 002).
- **FR-005**: Import MUST be entry-resilient: a single malformed entry is skipped
  and counted, not fatal (consistent with feature 002).
- **FR-006**: The user MUST choose the source format on import and the target
  format on export (no silent auto-detection in v1).
- **FR-007**: Export MUST surface the same clear-text warning as native export
  for every format (all four are unencrypted).
- **FR-008**: Format adapters MUST live in a UI-independent package depending only
  on the item model, so CLI and GUI share them (consistent with the project's
  UI/core decoupling).

### Key Entities

- **Codec**: A named, bidirectional adapter for one file format — marshals
  `[]items.Content` to bytes and unmarshals bytes to `[]items.Content` (with a
  skip count). The native cowbird format is one codec among several.
- **Format registry**: The set of available codecs, surfaced to the UI as a
  selectable list.

## Type-Mapping Reference (informative)

Exact field tables live in `plan.md`. The semantic correspondence is:

| cowbird   | 1Password (cat)   | Bitwarden (type) | Proton (type) | LastPass        |
|-----------|-------------------|------------------|---------------|-----------------|
| Login     | Login (001)       | login (1)        | login         | login row       |
| Card      | Credit Card (002) | card (3)         | creditCard    | secure note     |
| Note      | Secure Note (003) | secureNote (2)   | note          | secure note (sn)|
| Identity  | Identity (004)    | identity (4)     | identity      | secure note     |
| Password  | Password (005)    | login (1)*       | login*        | login row*      |
| Custom    | Secure Note (003)*| secureNote (2)*  | note*         | secure note*    |

`*` = no exact target type; mapped to the nearest type and documented as lossy.

## Assumptions

- Each foreign export is plaintext/unencrypted (matching what each tool emits for
  its unencrypted export option). Proton's PGP-encrypted ZIP and 1Password's
  encrypted formats are out of scope.
- The user selects the format explicitly; cowbird does not guess from file
  contents in v1.
- Re-import of the same file creates duplicates (no de-dup), as in feature 002.

## Out of Scope

- Auto-detecting the source format.
- Folders / vaults / collections, sharing relationships, attachments, passkeys,
  and TOTP secret validation — only item field data is migrated.
- Encrypted/passphrase-protected foreign files (Proton PGP ZIP, etc.).
- Importing item types with no field analog (e.g. SSH keys, documents); these are
  skipped or reduced to notes.
- CSV variants for Bitwarden/Proton/1Password (only the chosen format per vendor).
