# Implementation Plan: Key Import (Restore from Recovery File)

**Branch**: `007-key-import` | **Spec**: [spec.md](./spec.md)

## Technical Context

`crypto.ImportKey(data, passphrase) (*Identity, error)` already decrypts the
recovery file. The new work is a `core` function that performs the safety check
and the Vault writes, and an import form in the unlock window. No storage schema
or policy change.

## Architecture Overview

### `internal/core` — `ImportIdentity`

```go
var ErrIdentityMismatch = errors.New("recovery file is for a different identity")

func ImportIdentity(ctx, v *vault.Vault, data, exportPassphrase, newUnlockPassword []byte, force bool) (*crypto.Identity, error)
```

1. `crypto.ImportKey(data, exportPassphrase)` → identity (verifies the
   passphrase / file format; FR-002).
2. Safety check (FR-004/FR-005): `v.GetPublicKey(ctx, v.EntityID)`.
   - `ErrNotFound` → no published key, skip the check.
   - found and `pub != id.EncryptionPub` and `!force` → return
     `ErrIdentityMismatch` (the UI turns this into a confirm-and-retry).
   - found and equal → proceed.
3. Re-lock under the new unlock password and store: `crypto.LockIdentity(id,
   newUnlockPassword)` → `v.PutLockedIdentity` (FR-003/FR-007).
4. `v.DeletePrevLockedIdentity(ctx)` ignoring `ErrNotFound` — clear any stale
   rotation marker (FR-008).
5. `v.PutPublicKey(ctx, v.EntityID, id.EncryptionPub, v.DisplayName)` so the
   directory reflects the restored key (FR-006).
6. Return the identity; the caller wraps it in `core.NewApp` exactly like
   `InitIdentity`'s result.

Untested at the core layer (needs a live Vault), per the
`InitIdentity`/`ChangePassword` precedent; `crypto.ImportKey` round-trip is
already covered by `crypto/export_test.go`.

### `internal/ui` — import in the unlock window

`unlock.go` builds either a set-password or enter-password form after the
first-run check. Add:

- An **"Import recovery key…"** button beneath the submit button in both forms,
  which swaps `body` to the import form (the same single-slot `body` container
  already used for the connecting/first-run swap).
- **Import form** fields: a "Choose file…" button (`dialog.ShowFileOpen` →
  read all bytes from the `fyne.URIReadCloser`, remember them, show the chosen
  name in a label), export passphrase, new unlock password, confirm new unlock
  password, plus the advisory strength meter on the new password. A "Back"
  affordance returns to the password form.
- **Submit**: validate (file chosen; passphrase non-empty; new password
  non-empty and equal to confirm) on the main thread, then call
  `core.ImportIdentity(..., force=false)` in a goroutine. On `fyne.Do`:
  - success → `onUnlock(core.NewApp(v, id))`, close the window (same hand-off as
    the normal unlock path);
  - `errors.Is(err, core.ErrIdentityMismatch)` → `dialog.ShowConfirm` warning
    that the file is a different identity and items will be lost; on confirm,
    re-run with `force=true`;
  - other error → inline message, form stays open.
- Threading: capture widget values before launching; all widget/dialog updates
  via `fyne.Do`; file bytes read in the file-open callback (main thread) and
  captured.

The normal set/enter-password path is unchanged.

## Build Order

1. **core**: `ImportIdentity` + `ErrIdentityMismatch`.
2. **ui**: import button + import form + file-open wiring + mismatch confirm in
   `unlock.go`.
3. Docs: README status (flip key import to done), CLAUDE.md (core + ui/unlock
   notes, "On the horizon"), project-status memory.

## Testing

- Core is untested per precedent; `crypto.ImportKey` round-trip already covered.
- Manual against live Vault: (a) restore into a Vault with no identity → enter
  app, items readable; (b) forgotten-password restore with the *matching* file →
  new password works, items readable; (c) import a *non-matching* file → mismatch
  warning, and on confirm the identity is replaced; (d) wrong passphrase / bad
  file → clear error, nothing written; (e) confirm `identity.prev` is cleared if
  one was present.

## Open Items

- The mismatch check relies on the published pubkey; a user who never published
  one (extremely early record) gets no check. Acceptable — that path is the
  fresh-restore case anyway.
- No undo: importing the wrong file over a valid identity and confirming through
  the warning will strand items. The warning + fingerprint check are the
  mitigations; full zero-trust verification is out of scope (001 Decision 3).
- A future "import" entry could also live in the setup window for users starting
  from scratch, but the unlock window already covers the post-auth case.
