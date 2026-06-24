# Feature Specification: Favorites and Labels

**Feature Branch**: `012-favorites-and-labels`
**Status**: Draft
**Created**: 2026-06-24

## Summary

Cowbird stores items but offers no way to organize them beyond type filtering
and search. This feature adds two organizing primitives:

- **Favorites** — a per-user star on any item, surfaced as a quick filter and a
  pin to the top of the list.
- **Labels** — flat, multi-assignable tags (an item may carry zero or many),
  with user-defined names and optional colors, used to filter the list. Labels
  were chosen over single-home folders because a credential legitimately belongs
  to more than one grouping at once (e.g. "work" and "email"), they compose with
  the existing type filter and search rather than competing with them, and a
  flat tag set is far simpler to keep consistent inside an encrypted store than a
  hierarchical tree. (See plan.md, "Why labels, not folders".)

Both are **per-user, private organization**, not properties of the item itself.
They apply uniformly to items the user owns *and* to items shared *with* them,
they are visible only to the user who set them, and toggling them never rewrites
or re-distributes a shared item's envelope. This is achieved with a per-user
encrypted overlay record, independent of item content. (See plan.md, "Why an
overlay, not item content".)

## User Scenarios

### User Story 1 — Favorite an item (Priority: P1)

A user marks the handful of items they reach for daily so they surface first.

**Why this priority**: Favorites is the smallest, highest-frequency win and the
simplest slice of the overlay machinery everything else builds on.

**Acceptance Scenarios**:

1. **Given** a user viewing any item (owned or shared with them), **When** they
   toggle its favorite star, **Then** the item is marked favorite and remains so
   on next launch.
2. **Given** some items are favorited, **When** the user enables a
   "Favorites" filter, **Then** only favorited items are listed.
3. **Given** the default list ordering, **When** items are displayed, **Then**
   favorited items are pinned above non-favorited items.
4. **Given** a user favorites an item that is shared with them, **When** the
   owner or any other recipient views that same item, **Then** they do **not**
   see it as favorited (favorites are private to each user).

### User Story 2 — Create and assign labels (Priority: P1)

A user defines labels and tags items with one or more of them.

**Acceptance Scenarios**:

1. **Given** a user, **When** they create a label with a name (and optionally a
   color), **Then** the label exists and is available to assign to items.
2. **Given** an existing label, **When** the user assigns it to one or more
   items, **Then** each item displays that label, and an item may carry several
   labels at once.
3. **Given** an item shared *with* the user, **When** they assign a label to it,
   **Then** the label applies in their own view only and does not modify the
   shared item or affect the owner or other recipients.
4. **Given** assigned labels, **When** the user restarts the app, **Then** label
   definitions and assignments persist.

### User Story 3 — Filter by label (Priority: P1)

A user narrows the list to a chosen label.

**Acceptance Scenarios**:

1. **Given** items with various labels, **When** the user selects a label
   filter, **Then** only items carrying that label are listed.
2. **Given** a label filter is active, **When** the user also sets the type
   filter or types a search term, **Then** the filters compose (intersection).

### User Story 4 — Manage labels (Priority: P2)

A user renames, recolors, or deletes a label.

**Acceptance Scenarios**:

1. **Given** an existing label, **When** the user renames or recolors it,
   **Then** every item carrying it reflects the change.
2. **Given** a label assigned to several items, **When** the user deletes the
   label, **Then** it is removed from all items and from the available filters;
   the items themselves are untouched.

### User Story 5 — Organization survives item lifecycle (Priority: P3)

Stars and label assignments don't outlive the items they point at, and don't
leak across re-shares.

**Acceptance Scenarios**:

1. **Given** a favorited and labeled item, **When** the user deletes that item,
   **Then** its favorite and label assignments are discarded (no dangling
   organization for a non-existent item).
2. **Given** an item shared with the user that they labeled, **When** the owner
   revokes the share, **Then** the now-dead link's organization is not retained
   or re-applied if a *different* item is later shared.

## Requirements

### Functional Requirements

- **FR-001**: System MUST let a user mark/unmark any item they can read (owned or
  shared with them) as a favorite.
- **FR-002**: System MUST let a user define labels with a name and an optional
  color, and assign zero or more labels to any item they can read.
- **FR-003**: Favorites and label assignments MUST be private to the user who set
  them and MUST NOT be visible to the item's owner, to other recipients, or to
  the storage operator.
- **FR-004**: Setting or clearing a favorite or label on a shared item MUST NOT
  modify, re-key, or re-distribute that item's shared envelope.
- **FR-005**: Favorites and label assignments MUST persist across sessions and be
  available on any device where the user unlocks their identity.
- **FR-006**: The list MUST offer a favorites filter and a per-label filter, and
  these MUST compose with the existing item-type filter and text search
  (intersection semantics).
- **FR-007**: Favorited items MUST sort ahead of non-favorited items in the
  default list order.
- **FR-008**: Renaming or recoloring a label MUST apply everywhere it is shown;
  deleting a label MUST remove it from all items and filters without altering the
  items' contents.
- **FR-009**: Deleting an item MUST remove that item's favorite flag and label
  assignments from the user's organization record.
- **FR-010**: The organization record MUST be stored encrypted such that the
  storage operator cannot read label names, colors, or which items a user has
  favorited or labeled.
- **FR-011**: Organization data MUST be modeled independently of the Fyne UI so a
  future CLI can read and modify it.

### Key Entities

- **Label**: a user-defined tag — opaque ID, display name, optional color.
- **Item organization**: per-item, per-user metadata — a favorite flag and a set
  of assigned label IDs, keyed by the item's identifier (itemID for owned items,
  shareID for items shared with the user).
- **Organization record**: the single per-user collection of label definitions
  and per-item organization, stored encrypted-to-self.

## Assumptions

- The set of labels and favorited items per user is small (tens, not millions),
  so a single overlay record read/written whole is acceptable — matching the
  reserved per-user `pinned` record precedent.
- Cross-device concurrent edits to the organization record resolve last-writer-
  wins, consistent with the rest of cowbird's single-record state.
- Re-keying or re-sharing an owned item keeps its itemID, so the owner's own
  organization for it is preserved; a recipient's organization is keyed by
  shareID and is intentionally not carried across a revoke + fresh re-share.

## Out of Scope

- Hierarchical folders / nested labels (flat labels only; see plan.md rationale).
- Sharing or syncing labels *between* users (organization is strictly per-user).
- Smart/saved searches or rule-based auto-labeling.
- Bulk multi-select label assignment in the UI (single-item assignment first;
  may follow later).
- Per-label icons beyond a color.
