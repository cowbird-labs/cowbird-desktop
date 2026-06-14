# Implementation Plan: Key Export (Recovery File)

**Branch**: `006-key-export` | **Spec**: [spec.md](./spec.md)

## Technical Context

`crypto.ExportKey(id, passphrase) ([]byte, error)` already produces the
passphrase-protected JSON. The new work is small: a `core` function that gates
export behind the unlock password, and a UI flow (dialog + file save). No
storage changes, no `Store`/policy changes — nothing is written to Vault.

## Architecture Overview

### `internal/core` — `ExportIdentity`

```go
func ExportIdentity(ctx context.Context, app *App, unlockPassword, exportPassphrase []byte) ([]byte, error)
```

- `GetLockedIdentity` + `crypto.UnlockIdentity(unlockPassword)` to verify the
  unlock password (FR-002); the generic wrong-password error propagates.
- `crypto.ExportKey(id, exportPassphrase)` on the freshly unlocked identity
  (identical to `app.Identity`; using the just-verified one keeps export and
  authorization on the same record).
- Returns the file bytes. Writing the file is the UI's job (it owns the chosen
  location). This mirrors `ChangePassword`'s shape and stays untested at the
  core layer, per the `InitIdentity`/`ChangePassword` precedent (needs a live
  Vault to verify the password).

### `internal/ui` — export dialog + file save

- A third hamburger-menu item, **"Export Recovery Key…"**, alongside Change
  Password and Rotate Key.
- A custom dialog (the `password.go` pattern, so validation/auth errors keep the
  form open): a clear warning that this file is the only recovery path and must
  be stored safely offline, then three fields — current unlock password, export
  passphrase, confirm export passphrase — with the advisory strength meter on
  the export passphrase.
- Validation before any work: non-empty unlock password; export passphrase
  non-empty and equal to its confirmation (FR-003).
- Submit captures the field values on the main thread, then runs
  `core.ExportIdentity` in a goroutine (Vault round-trip to verify the
  password). On success, via `fyne.Do`, hide the dialog and open a file-save
  dialog (`dialog.NewFileSave`, default name `cowbird-recovery.json`); on the
  chosen `fyne.URIWriteCloser`, write the bytes and close, surfacing any write
  error (FR-005). On auth/validation failure, show the error inline and keep the
  dialog open.
- No item-list reload (nothing about the session changes).
- Threading rules as established: values captured before launch; widget/dialog
  updates via `fyne.Do`.

## Build Order

1. **core**: `ExportIdentity`.
2. **ui**: menu item + export dialog + file-save wiring (helper in
   `password.go`, which already holds the menu and the sibling dialogs).
3. Docs: README status, CLAUDE.md (core section + "On the horizon"),
   project-status memory.

## Testing

- `crypto.ExportKey`/`ImportKey` round-trip is already covered by
  `crypto/export_test.go`. `ExportIdentity` is a thin gate over it and the
  password verification needs a live Vault, so it follows the untested-core
  precedent.
- Manual against live Vault: export with the correct unlock password → a file is
  written; wrong unlock password → refused, no file; mismatched export
  passphrase → blocked; confirm the file is not plaintext (contains the
  `ExportedKey` JSON with `ciphertext`, not the raw key).

## Open Items

- **Import** is the natural sequel and is deferred. Its design needs deciding:
  where an imported key lands (re-lock under a new unlock password and write to
  Vault?), how it interacts with an existing Vault-stored identity, and whether
  it runs from the setup window, the unlock window, or both. Out of scope here.
- A future "you haven't exported a recovery key" nudge could encourage users to
  create one, but is not part of this feature.
- Re-export after key rotation is manual; a prompt offering it post-rotation is
  a possible later enhancement.
