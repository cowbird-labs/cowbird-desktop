# Implementation Plan: Share and Revoke UI

**Branch**: `003-share-ui` | **Spec**: [spec.md](./spec.md)

## Technical Context

Builds directly on 002: `Service.Share`/`Revoke` and owner-side ShareRecords
exist and are tested; the item UI has a detail view to host the sharing
section. The missing pieces are (a) human-readable identity, (b) a directory
listing, and (c) the UI itself.

## Architecture Overview

### Display names ride the auth result

Each auth method already receives a usable name during login; it is currently
discarded:

| Method | Source |
|---|---|
| Userpass | `resp.Auth.Metadata["username"]` |
| Token | `resp.Data["display_name"]` from the lookup-self call |
| AppRole | `resp.Auth.Metadata["role_name"]` |

`auth.Result` gains `DisplayName string`; each method populates it (empty on
renewal paths is fine — the renewal loop only updates EntityID). `Vault`
stores it as a `DisplayName` field, like `EntityID`.

### Names are published with the public key

The at-rest pubkey record becomes `{pub, name}`. Older records without `name`
unmarshal with an empty name (no migration). Publishing happens:

- at identity creation (as today), now with the name; and
- on every successful unlock (`core.InitIdentity`, returning-user path), which
  re-publishes the same key with the current name — pre-name entries
  self-heal, and renames in Vault propagate. One small KV write per login.

### Store and service additions

```go
// sharing — new type
type PublicKeyEntry struct {
	EntityID string
	Pub      [32]byte
	Name     string // advisory display name; may be empty
}
```

- `Store.PutPublicKey` gains a `name` parameter (signature change; three
  implementations: vault, in-memory test store, and the core call site).
- `Store.ListPublicKeys(ctx) ([]PublicKeyEntry, error)` — kvList `pubkeys/`
  plus a read per entry.
- `Service.Directory(ctx)` — pass-through to `ListPublicKeys`. The UI also
  uses it to resolve entity IDs to names everywhere ("shared by", access
  list).
- `Service.ListShareRecords(ctx, itemID)` (from 002) backs the access list.
- No changes to `Share`/`Revoke` themselves.

### UI

- **Name resolution**: `loadRows` also fetches the directory and returns a
  `map[entityID]displayName`; `mainWindow` keeps it for the session
  (refreshed on every reload). Helper `displayName(id)` falls back to an
  8-char entity-ID prefix, and `pickerLabel` appends the prefix when two
  entries share a name (FR-007).
- **Detail view, owned items**: a "Sharing" section under the fields. Loaded
  asynchronously (ShareRecords is a Vault call): shows "Not shared" or one row
  per recipient — name + revoke icon button. Revoke confirms
  (`dialog.ShowConfirm`), runs `Service.Revoke` in a goroutine, refreshes the
  section.
- **Share dialog**: "Share…" button opens `dialog.ShowCustomConfirm` with a
  `widget.Select` of eligible users (directory minus self minus current
  recipients). Empty eligible set → disabled state with explanation. Confirm
  runs `Service.Share` in a goroutine, then refreshes the sharing section.
- **Shared (received) items**: subtitle becomes "shared by <name>"; no
  sharing controls (recipients cannot re-share — and the policy would refuse
  the envelope write anyway).
- Threading rules as established: Vault I/O in goroutines, widget updates via
  `fyne.Do`, values captured before launch.

## Build Order

1. **auth**: `Result.DisplayName` + population in all three methods;
   `Vault.DisplayName`.
2. **sharing/vault/core**: `PublicKeyEntry`, `PutPublicKey` signature change,
   `ListPublicKeys`, `Service.Directory`; re-publish on unlock in
   `core.InitIdentity`. Extend the in-memory store; tests for directory
   listing and name fallback.
3. **UI**: names map in the load path, "shared by" labels, sharing section
   with access list + revoke, share dialog.
4. Docs: README status, CLAUDE.md.

## Open Items

- AppRole entities are machine identities; sharing *to* one is legitimate
  (e.g. a CI reader) and needs no special casing.
- Renaming a Vault user changes the published name on next unlock; old name
  may linger in other users' open windows until their next reload. Accepted.
- A future "verify this contact" affordance (001 research.md Decision 3)
  would hang off the same directory entries.
