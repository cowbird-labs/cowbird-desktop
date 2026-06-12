# Feature Specification: Share and Revoke UI

**Feature Branch**: `003-share-ui`
**Status**: Draft
**Created**: 2026-06-12

## Summary

The sharing machinery (envelopes, inbox protocol, owner-side ShareRecords) is
complete and tested, but has no UI: today an item can only be shared from test
code. This feature surfaces it: an owner can share an item with another user
picked from a human-readable directory, see who currently has access to each
of their items, and revoke any recipient's access — all from the item detail
view. To make the directory human-readable, users publish a display name
alongside their public key; entity IDs (opaque UUIDs) stop appearing in the UI
except as a disambiguating suffix.

## User Scenarios

### User Story 1 — Share an item with another user (Priority: P1)

From an item they own, a user opens a share dialog, picks another cowbird user
by name from a directory list, and shares. The recipient sees the item in
their own list after their next refresh.

**Why this priority**: This is the feature. Everything else here supports it.

**Acceptance Scenarios**:

1. **Given** Paul owns an item and Aubry has a published public key, **When**
   Paul opens the item's share dialog, **Then** he sees Aubry listed by name
   (not by UUID) and can share the item with her.
2. **Given** Paul has shared the item with Aubry, **When** Aubry refreshes her
   list, **Then** the item appears marked as shared by Paul (by name).
3. **Given** the share dialog is open, **Then** Paul himself and users who
   already have access to this item are not offered as choices.
4. **Given** a share attempt fails (Vault unreachable), **Then** the error is
   surfaced and no phantom recipient appears in the access list.

### User Story 2 — See who has access (Priority: P1)

The detail view of an owned item shows the item's current recipients by name.

**Acceptance Scenarios**:

1. **Given** an item shared with two users, **When** the owner views it,
   **Then** both recipients are listed by name.
2. **Given** an item shared with nobody, **When** the owner views it, **Then**
   the sharing section shows it is not shared.
3. **Given** an item shared with the user (not owned), **When** they view it,
   **Then** no sharing controls are shown (recipients cannot re-share).

### User Story 3 — Revoke a recipient's access (Priority: P1)

Next to each recipient in the access list, the owner can revoke, with
confirmation.

**Acceptance Scenarios**:

1. **Given** an item shared with Aubry, **When** Paul revokes her access and
   confirms, **Then** she disappears from the access list and the shared
   envelope is gone (she cannot decrypt the content even via the raw store).
2. **Given** the confirmation prompt, **When** Paul declines, **Then** nothing
   changes.
3. **Given** a revoked share, **When** Aubry's client next processes its
   inbox, **Then** the item disappears from her list.
4. **Given** Paul revoked Aubry's access, **When** he shares the same item
   with her again, **Then** she regains access (revoke is not permanent ban).

### User Story 4 — Users appear by name, not UUID (Priority: P2)

Display names are published with public keys so directory entries, access
lists, and "shared by" labels are human-readable.

**Acceptance Scenarios**:

1. **Given** a user authenticates and creates their identity, **Then** their
   published public-key entry carries a display name derived from their auth
   identity (userpass username, token display name, or AppRole role name).
2. **Given** a user whose key was published before names existed, **When**
   they next unlock, **Then** their entry is re-published with a name
   (self-healing; no migration step).
3. **Given** two users with the same display name, **When** they appear in a
   picker or list, **Then** they are disambiguated (entity-ID prefix suffix).
4. **Given** a directory entry with no name, **Then** the entity-ID prefix is
   shown rather than an empty string.

## Requirements

### Functional Requirements

- **FR-001**: The detail view of an owned item MUST offer a share action that
  lists all other users from the public-key directory by display name,
  excluding the owner and users who already have access.
- **FR-002**: Sharing MUST use the existing `Service.Share` protocol; the
  recipient needs no new client behavior (inbox processing already handles it).
- **FR-003**: The detail view of an owned item MUST show its current
  recipients (from ShareRecords) by display name.
- **FR-004**: Each listed recipient MUST have a revoke action behind an
  explicit confirmation; revocation deletes the shared envelope (the security
  action) and notifies the recipient's client.
- **FR-005**: Shared (received) items MUST show the owner's display name in
  the detail view and offer no sharing controls.
- **FR-006**: Public-key directory entries MUST carry a display name,
  published at identity creation and refreshed at unlock (so pre-name entries
  self-heal). Entries without a name MUST fall back to an entity-ID prefix.
- **FR-007**: Duplicate display names MUST be disambiguated wherever names are
  shown for selection.
- **FR-008**: Share and revoke failures MUST surface as user-readable errors;
  the access list MUST reflect only successfully recorded state.

### Key Entities

- **Directory entry** (new): a published public key plus display name —
  `PublicKeyEntry{EntityID, Pub, Name}`. The at-rest pubkey record gains a
  `name` field (older records without it remain readable).
- **Access list**: the owner's ShareRecords for an item, displayed by
  recipient name. Reuses `sharing.ShareRecord` unchanged.
- **DisplayName** (new on `auth.Result`): the human-readable identity each
  auth method reports (userpass username, token display name, AppRole role
  name).

## Assumptions

- Display names live in the world-readable pubkey directory. Names are
  identifying by design (that is their purpose); a deployment's users are
  already known to each other, so this leaks nothing beyond what the operator
  and users already know.
- Names are advisory, not authenticated: under the soft trust model (001
  research.md Decision 3), the operator could substitute names just as they
  could substitute keys. Accepted.
- One share per recipient per item at a time; re-sharing after revoke creates
  a new share. The picker prevents duplicate concurrent shares.

## Out of Scope

- Sharing with multiple recipients in one dialog action (share twice instead).
- Write access for recipients (shares are read-only by design).
- Groups, roles, or share-all.
- Contact verification / TOFU (deferred in 001).
- User-chosen display names or renaming UI (names follow the auth identity).
- Notifications to the recipient beyond the existing inbox mechanism.
