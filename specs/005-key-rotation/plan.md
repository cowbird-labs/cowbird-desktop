# Implementation Plan: Key Rotation

**Branch**: `005-key-rotation` | **Spec**: [spec.md](./spec.md)

## Technical Context

All crypto and storage primitives exist. The new work is (a) a transitional
slot for the old key, (b) a bulk re-key operation in `sharing.Service`, and (c)
orchestration in `core` that is idempotent and resumes at unlock. No `Store`
interface change is required — `Rekey` uses only existing methods
(`ListItems`, `PutItem`, `GetPublicKey`, `ListShareRecords`,
`PutSharedEnvelope`, `SendMessage`). The transitional slot uses Vault-specific
identity methods (not part of `Store`), like the existing
`Get/PutLockedIdentity`.

## Why a transitional slot, and the ordering invariant

Full re-key re-encrypts each item under a fresh item key, so once an item is
rewritten only the **new** key can read it. A crash partway must not strand
data. The invariant: **the old key must stay recoverable until every owned item
is migrated.**

The old key is kept (password-locked) at `users/<entityID>/identity.prev`. Its
presence is the rotation-in-progress flag. Write order on a fresh rotation:

1. `PutPrevLockedIdentity(old)` — stage the old key (canonical still = old).
2. `PutLockedIdentity(new)` — canonical becomes new.

- Crash between 1 and 2: canonical = old, prev = old (same fingerprint).
  Nothing migrated. Resume sees prev.fingerprint == canonical.fingerprint →
  aborted-before-commit → just delete prev. No data touched.
- Crash after 2 (during re-key): canonical = new, prev = old (differ). Resume
  finishes migration using both keys, then deletes prev.

Only after **all** items are migrated and shares re-distributed is prev deleted
(FR-007) — that is the point the old key is destroyed.

## Architecture Overview

### `internal/vault/identity.go` — transitional slot

```go
func (v *Vault) GetPrevLockedIdentity(ctx) (*crypto.LockedIdentity, error) // ErrNotFound when absent
func (v *Vault) PutPrevLockedIdentity(ctx, locked) error
func (v *Vault) DeletePrevLockedIdentity(ctx) error // ignore ErrNotFound at call sites
```

Path `users/<entityID>/identity.prev`, same `{"v": json}` wrapper as the
canonical identity via `kvRead`/`kvWrite`/`kvDelete`.

### `internal/sharing/service.go` — `Rekey`

```go
func (s *Service) Rekey(ctx, oldPriv, newPriv, newPub [32]byte) error
```

Deliberately takes explicit key material (does not read `s.identity`) so the
caller controls which keys are in play. For each envelope from `ListItems`:

- **Migrate the owned item** (`rekeyOwnedItem`): if it already opens with
  `newPriv` it is done — return its current item key (idempotent resume). Else
  unwrap the item key with `oldPriv`, `crypto.Open` the content, generate a
  fresh `crypto.NewItemKey`, `crypto.Seal` the same plaintext under it, wrap to
  `newPub` as the sole `Recipients` entry, `PutItem`. Returns the migrated
  envelope + item key. (No `items.Decode`/`Encode` — the plaintext is rewrapped
  as-is; content is unchanged.)
- **Re-distribute shares** (`redistributeShares`): for each `ShareRecord` of the
  item, fetch the recipient's *current* `GetPublicKey`, wrap the new item key to
  it, rewrite the shared envelope (`PutSharedEnvelope`, same shareID), and
  `SendMessage` a fresh share message carrying the new wrapped key and the new
  `EnvVersion`. The recipient's existing `ProcessInbox` updates their SharedLink
  (version is higher, so it is not skipped). Idempotent: re-running rewrites the
  same envelope and re-sends a message the recipient consumes harmlessly.

Item IDs and share IDs are preserved throughout (FR-010).

### `internal/core/core.go` — orchestration + resume

```go
func RotateKey(ctx, app *App, password []byte) error
```

- Load + unlock canonical with `password` (verifies it; generic error on
  mismatch).
- **If prev exists** → a rotation is already in progress; call
  `completeInterruptedRotation` and refresh `app.Identity`/`app.Service` from the
  (already-new) canonical. (In-session retry path.)
- **Else fresh** → `crypto.NewIdentity`; stage old (`PutPrevLockedIdentity`),
  commit new (`PutLockedIdentity`); then `completeInterruptedRotation`; then
  swap `app.Identity = new` and rebuild `app.Service`.

```go
func completeInterruptedRotation(ctx, v *vault.Vault, canonical *crypto.Identity, password []byte) error
```

- `GetPrevLockedIdentity`; `ErrNotFound` → nothing to do.
- Unlock prev → old identity. If `old.Fingerprint == canonical.Fingerprint`,
  this is the aborted-before-commit case → delete prev, done.
- Otherwise publish the new public key (`PutPublicKey`, future shares target it),
  build a throwaway `sharing.NewService(entityID, canonical, v)`, run
  `Rekey(old.Priv, canonical.Priv, canonical.Pub)`, then
  `DeletePrevLockedIdentity`.

**Resume at unlock**: `InitIdentity`'s returning-user path calls
`completeInterruptedRotation(ctx, v, id, password)` right after unlocking and
before the existing pubkey re-publish. So an interrupted rotation finishes
before the main window is shown (FR-005, US3.2), with both keys available.

### `internal/ui` — entry point

- A second hamburger-menu item, **"Rotate Key…"**, next to Change Password.
- A confirm dialog (`dialog`) stating this is for compromise recovery, that
  other open sessions must be closed first, and that items shared *with* the
  user will need owners to re-share. Then a password prompt (reuse the custom
  dialog pattern from `password.go`).
- Submit runs `core.RotateKey` in a goroutine (lots of Vault I/O), with a busy
  status; on success, `m.reload()` via `fyne.Do` (item keys changed; reload to
  be safe). Errors surface inline / in the status bar and rotation is retryable.
- Threading rules as established: capture values on the main thread, widget
  updates via `fyne.Do`.

## Build Order

1. **vault**: `Get/Put/DeletePrevLockedIdentity` in `identity.go`.
2. **sharing**: `Rekey` + `rekeyOwnedItem` + `redistributeShares`; add a
   `service_test.go` case (Alice rotates with an owned item and a share to Bob;
   assert Alice still reads her item, the item key/ciphertext changed, and Bob —
   after `ProcessInbox` — still reads it; assert `Rekey` run twice is
   idempotent).
3. **core**: `RotateKey` + `completeInterruptedRotation`; wire resume into
   `InitIdentity`.
4. **ui**: menu item + confirm/password dialogs.
5. Docs: README status, CLAUDE.md (core + vault sections, "On the horizon"),
   project-status memory.

## Testing

- `sharing` gets a real unit test via the in-memory store (covers the data
  path: owned re-key, recipient retention, idempotent resume).
- `core` orchestration is untested at unit level (needs a live `*vault.Vault`),
  per the `InitIdentity`/`ChangePassword` precedent.
- Manual against live Vault: create items + a share, rotate, confirm own items
  still open and the recipient still reads after inbox processing; kill the app
  mid-rotation (or simulate by leaving prev) and confirm unlock completes it.

## Open Items

- **Concurrent sessions**: no active locking; the pre-rotation warning is the
  only mitigation (001 research.md Decision 12). A stale session writing under
  the old key during rotation can still corrupt — documented, not solved.
- **KV v2 history**: prior versions of rotated envelopes persist in mount
  history, decryptable only with the destroyed old key. Destroying old versions
  on the item/shared paths is a possible follow-up (the self-subtree policy
  already grants delete; shared-path destroy needs checking).
- **In-memory swap race**: `RotateKey` mutates `app.Identity`/`app.Service` from
  a worker goroutine; the UI treats rotation as a modal operation and reloads on
  completion. A fuller fix would funnel the swap through the main thread.
- Items shared *with* the user are intentionally not re-keyed here (US4); a
  future "request re-share" affordance could nudge owners.
