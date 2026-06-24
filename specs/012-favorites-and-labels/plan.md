# Implementation Plan: Favorites and Labels

**Branch**: `012-favorites-and-labels` | **Spec**: [spec.md](./spec.md)
**Status**: Draft

Same stack as the rest of the project (Go 1.26, Fyne, Vault KV v2). Favorites and
labels are a per-user encrypted overlay joined onto the existing item list; no
item envelopes change.

## Two decisions, recorded

### Why an overlay, not item content

Item content lives in the end-to-end-encrypted envelope, and the UI treats items
shared *with* the user as read-only — a recipient cannot write the owner's
envelope. Storing favorites/labels as fields on `items.Content` would therefore
make it impossible to organize shared items, and every star toggle on an owned
*shared* item would rewrite and re-distribute its envelope to all recipients
(`sharing.Service.UpdateItem`), leaking the user's private organization to the
owner and other recipients. Instead, organization is a separate per-user record
encrypted to the user's own key. It works identically for owned and shared rows
(keyed by itemID / shareID), never touches a shared envelope, and stays private.
This matches the reserved `users/<entityID>/pinned` self-encrypted-record
precedent already in the storage layout.

### Why labels, not folders

Flat, multi-assignable labels beat single-home folders here: a credential
legitimately belongs to several groupings at once; labels compose with the
existing type filter and search (intersection) instead of replacing them; and a
flat `itemID → []labelID` map is trivial to keep consistent inside one encrypted
blob, whereas a folder tree needs parent pointers, reparenting, and cascade-on-
delete with no real payoff for a credential store. Hierarchy is out of scope.

## Vault storage

One new KV v2 path, in the user's own subtree (covered by the existing self-
subtree policy rule — no policy change needed):

```
cowbird/data/users/<entityID>/organization   # per-user encrypted org record
```

Written whole on each change (the data is small; last-writer-wins across devices,
consistent with the rest of cowbird). Stored as a `crypto.SelfSealed` blob — the
plaintext JSON of the `Organization` record never reaches Vault.

## Crypto: seal-to-self (`internal/crypto`)

The organization record is encrypted under the user's *in-memory* keypair (no
password prompt per toggle), reusing the existing envelope primitives. New in
`crypto/self.go`:

```go
// SelfSealed is content encrypted to the holder's own X25519 key.
type SelfSealed struct {
	EphemeralPub []byte `json:"ephemeral_pub"` // wrap ECDH ephemeral
	WrapNonce    []byte `json:"wrap_nonce"`
	WrappedKey   []byte `json:"wrapped_key"`   // content key wrapped to own pub
	Nonce        []byte `json:"nonce"`         // content nonce
	Ciphertext   []byte `json:"ciphertext"`
}

func SealToSelf(id *Identity, plaintext []byte) (*SelfSealed, error)
func OpenFromSelf(id *Identity, b *SelfSealed) ([]byte, error)
```

`SealToSelf`: `NewItemKey()` → `Seal(itemKey, plaintext)` for content →
`WrapKey(id.EncryptionPub, itemKey)` for the key. `OpenFromSelf`:
`UnwrapKey(id.EncryptionPriv, …)` then `Open`. Authenticated (XChaCha20-Poly1305),
so operator tampering fails closed. This helper is reusable by the future
`pinned` record.

## Model: `internal/organization` (new, UI-independent)

Pure types + mutation helpers so the planned CLI can reuse them (mirrors how
`items`/`transfer` stay UI- and Vault-independent):

```go
type Label struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"` // optional hex, e.g. "#3b82f6"
}

type ItemMeta struct {
	Favorite bool     `json:"favorite,omitempty"`
	Labels   []string `json:"labels,omitempty"` // label IDs
}

type Organization struct {
	Version int                 `json:"version"` // schema version, starts at 1
	Labels  []Label             `json:"labels,omitempty"`
	Items   map[string]ItemMeta `json:"items,omitempty"` // itemID/shareID → meta
}
```

Methods (all pure, operate in memory; caller persists):
- `(*Organization) ToggleFavorite(id string)`, `IsFavorite(id) bool`
- `AddLabel(name, color) (Label, error)` (UUID id), `RenameLabel`, `RecolorLabel`,
  `DeleteLabel(id)` — `DeleteLabel` also strips the id from every `ItemMeta`
- `AssignLabel(id, labelID)`, `UnassignLabel(id, labelID)`, `LabelsOf(id) []string`
- `Forget(id string)` — drop an item's meta entirely (on item delete)
- `Prune(liveIDs map[string]bool) bool` — drop meta for ids no longer present;
  returns whether anything changed (lazy cleanup of dead shares / deleted items)
- empty `ItemMeta` entries are deleted, not left blank, to keep the map tight.

`JSON()`/`ParseOrganization([]byte)` round-trip; a nil/absent record decodes to
an empty `Organization{Version: 1}`.

## Orchestration: `internal/core` + `internal/vault`

- `vault/organization.go`: `GetOrganization(ctx) (*crypto.SelfSealed, error)`
  (treats `ErrNotFound` as "none yet"), `PutOrganization(ctx, *crypto.SelfSealed)`
  at `users/<entityID>/organization`. Vault stays independent of the
  `organization` package — it only handles the sealed blob.
- `core/organization.go`:
  - `LoadOrganization(ctx) (*organization.Organization, error)` — get blob,
    `OpenFromSelf`, parse; absent → fresh empty record.
  - `SaveOrganization(ctx, *organization.Organization) error` — `JSON` →
    `SealToSelf` → `PutOrganization`.
  - `App.DeleteItem`/shared-link deletion paths call `Forget` + save so FR-009
    holds (or the UI does it explicitly around the existing delete call — decide
    during implementation; keep the prune lazy as a backstop).

## UI: `internal/ui`

`loadRows` (`model.go`) gains an organization load and annotates rows:

- `itemRow` gets `Favorite bool` and `Labels []string` (resolved label IDs kept
  on the row; names/colors looked up from the label set for display).
- `loadRows` returns the `*organization.Organization` (or at least its label
  definitions) alongside `rows`/`names`; after building rows it calls
  `Prune(liveIDs)` and, if changed, saves back (lazy cleanup). `mainWindow` holds
  the current `org` for filter population and mutation.

Controls and interactions:
- **Favorite star**: a toggle in the detail pane (`detail.go`) for every readable
  row, owned or shared; optional star glyph in the list row. Toggling calls
  `ToggleFavorite` + `SaveOrganization` (off-main-thread save, `fyne.Do` for the
  refresh), then re-sorts.
- **Sort** (`model.go` sort step): favorites first, then existing title order
  (FR-007).
- **Label assignment**: a "Labels" affordance in the detail pane (works on shared
  items too, since it never touches the envelope) — a popup to check/uncheck
  existing labels and create a new one inline.
- **Filters** (`app.go`): add a favorites toggle and a label `widget.Select`
  (an "All labels" sentinel like `allTypesOption`, populated from `org.Labels`)
  next to `search`/`typeFilter`. Extend `itemRow.matchesFilter` (and
  `applyFilter`) to AND in the favorite and label predicates (FR-006).
- **Label manager**: a "Manage labels…" entry in `showMainMenu` opening a dialog
  to rename/recolor/delete labels (FR-008); deletions propagate via
  `DeleteLabel`.

## Build Order (risk-first)

1. `crypto/self.go`: `SealToSelf`/`OpenFromSelf` + round-trip test.
2. `internal/organization`: model + pure mutation/prune helpers, with tests
   (favorite toggle, assign/unassign, delete-label strips assignments, prune,
   JSON round-trip incl. empty/absent).
3. `vault/organization.go` + `core/organization.go`: load/save orchestration.
4. UI: row annotation + favorites (star, sort, filter) first — smallest slice —
   then label assignment, label filter, and the label manager dialog.

## Tests

- `crypto`: `SealToSelf` → `OpenFromSelf` round-trips; a different identity (or
  tampered ciphertext) fails to open.
- `organization`: favorite toggle is idempotent-by-value; assign/unassign dedupes;
  `DeleteLabel` removes the id everywhere; `Prune` drops only dead ids and reports
  change; empty meta entries are collected; absent/nil JSON → empty record.
- Per project precedent, the Vault/core/UI layers are exercised indirectly (Vault
  needs a live server; Fyne UI is not unit-tested).

## Notes / follow-ups

- Re-keying/re-sharing an owned item keeps its itemID, so owner organization
  survives rotation; a recipient's shareID-keyed meta is intentionally dropped on
  revoke (a fresh re-share is a new shareID) — `Prune` handles the cleanup.
- Bulk multi-select assignment and sharing labels between users are out of scope
  (see spec).
- The reserved `pinned` record can later adopt `SelfSealed`.
