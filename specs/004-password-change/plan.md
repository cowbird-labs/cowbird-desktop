# Implementation Plan: Change Unlock Password

**Branch**: `004-password-change` | **Spec**: [spec.md](./spec.md)

## Technical Context

All crypto primitives already exist and are tested. `crypto.LockIdentity`
generates a fresh salt and seals the private key under an Argon2id-derived key;
`crypto.UnlockIdentity` reverses it and returns a generic error on a wrong
password. `vault.GetLockedIdentity`/`PutLockedIdentity` read and write the
identity at `users/<entityID>/identity`. The only missing pieces are a service
function that composes these and a UI entry point. No new storage paths, no
policy change, no schema change.

## Architecture Overview

### Service layer (`internal/core`)

Add one function mirroring `InitIdentity`'s style and signature conventions:

```go
func ChangePassword(ctx context.Context, v *vault.Vault, oldPassword, newPassword []byte) error
```

Steps:

1. `GetLockedIdentity` — load the current at-rest identity. `ErrNotFound`
   becomes a clear "no identity to change" error (should not happen from the
   main window, but handled).
2. `crypto.UnlockIdentity(locked, oldPassword)` — verifies the current password
   (FR-002); its generic error propagates unchanged.
3. `crypto.LockIdentity(id, newPassword)` — re-wrap with a fresh salt (FR-004).
4. `PutLockedIdentity(relocked)` — single write replaces the stored identity
   (FR-007: no destructive pre-step, so a failure here leaves the old record
   intact).

The in-memory `App.Identity` is the same keypair and is left untouched, so the
session stays live (FR-006). The function deliberately does not take or mutate
`*core.App` — it only needs the Vault handle and the two passwords, and
re-deriving from the stored record (rather than re-locking the in-memory
identity) is what verifies the old password against the real at-rest data.

### UI (`internal/ui`)

- **Entry point**: a new toolbar action on the main window
  (`NewMainWindow`, `app.go`) — a key/settings icon opening the change-password
  dialog. Placed after the existing add/refresh actions and the spacer.
- **Dialog**: `dialog.NewForm` (or `ShowCustomConfirm`) with three
  `widget.NewPasswordEntry` fields — current, new, confirm — plus the advisory
  strength bar reused from `unlock.go`/`strength.go` (`passwordStrength`).
  - Client-side validation before submit: non-empty current, new == confirm,
    new != current (FR-003).
  - Submit captures the three values on the main thread, then runs
    `core.ChangePassword` in a goroutine (Vault I/O off the main thread).
  - Result via `fyne.Do`: success toast/info dialog and close; failure shows the
    error inline and leaves the dialog open for retry (FR-008).
- No reload of the item list is needed — contents are unchanged. The session
  continues with the same `App`.
- Threading rules as established: capture widget values before launching the
  goroutine; all widget/dialog updates from the goroutine via `fyne.Do`.

## Build Order

1. **core**: `ChangePassword` in `core.go`.
2. **ui**: toolbar action + change-password dialog in `app.go` (helper in a new
   `password.go` if `app.go` grows unwieldy), reusing `passwordStrength`.
3. Docs: README status / CLAUDE.md "On the horizon" (move password change from
   pending to done); update the project-status memory.

## Testing

- The composition is thin and the underlying lock/unlock round-trip is already
  covered by `crypto` tests (seal/open, wrong-password generic error, fresh salt
  per lock). `core.InitIdentity` is likewise untested because it needs a live
  `*vault.Vault`; `ChangePassword` follows that precedent.
- If a focused regression guard is wanted without a live Vault, the candidate is
  a `crypto`-level test asserting that re-locking an identity with a new password
  yields a record the new password opens and the old password rejects — but that
  is exercising already-tested primitives, so it is optional.
- Manual verification against the live Vault (the project's norm for the
  Vault-touching paths): change password, confirm session stays live, relaunch,
  confirm new password works and old fails.

## Open Items

- KV v2 version history retains the prior locked identity; it remains
  decryptable only with the old password. If that is later deemed undesirable,
  destroy-old-versions on the identity path would be a follow-up (needs the
  `delete`/`destroy` capability already held on the self subtree).
- No concurrent-session coordination: safe here because the keypair is
  unchanged and a stale session keeps working under the old in-memory key. This
  is explicitly the property that distinguishes password change from key
  rotation.
