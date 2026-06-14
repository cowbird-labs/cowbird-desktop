# cowbird

A Go password manager that uses HashiCorp Vault as its storage backend. No server component of its own. GUI is Fyne-based. Targets desktop (macOS/Linux/Windows) and eventually mobile (Android/iOS).

- Repo: https://github.com/avitacco/cowbird
- App ID: `co.avitac.cowbird`
- Go 1.26

## Architecture

Core logic is decoupled from the UI so a CLI interface can be added later. Dependency injection is hand-coded in `main.go` (Wire is not used; not warranted at this scale).

Internal packages:

- `config` — configuration loading and saving
- `credentials` — credential model and storage
- `auth` — Vault authentication methods
- `vault` — Vault client wrapper; also implements `sharing.Store`
- `crypto` — keypair generation, XChaCha20-Poly1305, X25519 wrapping, Argon2id KDF, key export/import
- `items` — item content types and JSON codec
- `sharing` — envelope crypto, inbox protocol, share/revoke service
- `core` — application state (`App`), identity initialisation
- `ui` — Fyne UI

Key dependencies: `fyne.io/fyne/v2`, `github.com/99designs/keyring`, `hashicorp/vault-client-go` (v0.4.3), `spf13/viper`, `creasty/defaults`, `golang.org/x/crypto`.

### auth

`Method` interface with three implementations: `Userpass`, `Token`, `AppRole`. Each handles `Authenticate` and `Renew`. `Result` carries `DisplayName` (userpass username, token `display_name`, AppRole `role_name`; empty on renewal paths), stored as `Vault.DisplayName` and published with the public key.

### vault

`Vault` struct accepts an `auth.Method`. Stores the token as a struct field (not read back from client config, since `v.client.Configuration().Token` does not exist in vault-client-go v0.4.3). Runs background token renewal on a cancellable context.

`VerifyMount` lists the user's own KV metadata path and treats a 404 as success, because `MountsReadConfiguration` is not permitted by cowbird's Vault policy.

`*Vault` implements `sharing.Store` (`vault/store.go`). All KV writes store `{"v": "<json-string>"}` wrappers. Hard delete uses `KvV2DeleteMetadataAndAllVersions`. `vault/identity.go` adds `GetLockedIdentity`/`PutLockedIdentity` at path `users/<entityID>/identity`, plus `Get`/`Put`/`DeletePrevLockedIdentity` at `users/<entityID>/identity.prev` — the transitional slot holding the old key during key rotation (its presence is the rotation-in-progress flag; see `core.RotateKey`).

KV v2 paths used:
- `users/<entityID>/items/<itemID>` — owner's own items
- `users/<entityID>/identity` — locked identity (encrypted keypair)
- `users/<entityID>/identity.prev` — transitional old locked identity during key rotation (deleted on completion)
- `users/<entityID>/links/<shareID>` — durable SharedLink records
- `users/<entityID>/shares/<shareID>` — owner's ShareRecords (outgoing shares)
- `pubkeys/<entityID>` — public encryption keys + display names (`pubkeyRecord{pub, name}`; pre-name records unmarshal with empty name)
- `shared/<ownerEntityID>/<shareID>` — shared item envelopes
- `inbox/<recipientEntityID>/<msgID>` — transient share/revoke messages

### config

Viper + `mapstructure` + `creasty/defaults` for struct-tag-based defaults. Vault address and mount path are stored in TOML. Auth credentials are stored in the OS keyring via a `CredentialStore` interface (`Get`/`Set`/`Delete`), backed by `99designs/keyring` on desktop/CLI and stubbed on mobile (separated by build tags).

`Save(cfg Config) error` is a reusable function that uses `viper.ConfigFileUsed()`.

### crypto

Argon2id KDF with HKDF domain separation: `DeriveUnlockKey` uses info `"cowbird-unlock-v1"` and `DeriveWrapKey` (internal to `wrap.go`) uses `"cowbird-wrap-v1"`. Argon2id params: time=3, memory=64MB, threads=4, keyLen=32.

XChaCha20-Poly1305 for item content and key wrapping. `Seal`/`Open` in `aes.go`. `Open` returns a generic `"decryption failed"` error to avoid leaking failure mode.

X25519 keypairs via `crypto/ecdh` (Go stdlib since 1.20) — avoids manual scalar clamping. `WrapKey` (in `wrap.go`) uses ephemeral ECDH + HKDF to derive a per-wrap key; `EphemeralPub`, `Nonce`, and `Wrapped` are all stored explicitly.

`Identity` holds `EncryptionPub/EncryptionPriv [32]byte` and Ed25519 fields (deferred). `LockIdentity`/`UnlockIdentity` encrypt the private key with an Argon2id-derived key. `ExportKey`/`ImportKey` in `export.go` produce a passphrase-protected JSON blob (`ExportedKey{Version, Salt, Nonce, Ciphertext}`).

`Fingerprint` is hex-encoded SHA-256 of `EncryptionPub`.

### items

One `Content` interface with concrete typed structs: `Login`, `Card`, `Note`, `Identity`, `Password`, `Custom`. Each carries `CustomFields []Field` for arbitrary extra fields.

`Encode`/`Decode` in `codec.go` use a `{"type": "...", "data": {...}}` envelope for type-dispatch. `Decode` uses a generic helper `decodeAs[T Content]`.

### sharing

`Store` interface (`store.go`) defines all Vault operations used by the package — implemented by `*vault.Vault`.

`envelope.go`: `NewEnvelope` generates a random item key, encrypts content with XChaCha20-Poly1305, wraps the item key to the owner's X25519 public key, and returns the envelope. `OpenEnvelope` finds the owner's wrapped key and decrypts. `WrapKeyForRecipient` adds a recipient's wrapped key.

`service.go`: `Service{entityID, identity, store}` drives create/open/update/delete/list/share/revoke/processInbox. `ProcessInbox` is idempotent: write/remove SharedLink first, then delete the inbox message, so partial failures self-heal. Ordering tiebreaker is the KV v2 `env_version` (server-assigned monotonic version).

`Share` writes an independent *copy* of the envelope per share (new shareID, same ciphertext/item key) and records each outgoing share as a `ShareRecord` in the owner's subtree. `UpdateItem` reuses the existing item key (fresh nonce — recipients' wrapped keys stay valid) and rewrites every shared envelope found via ShareRecords. `DeleteItem` revokes all outstanding shares (envelope delete → revoke message → record delete) before deleting the owned envelope. `Revoke` and `DeleteSharedLink` are idempotent so partial failures are retryable.

`InboxEntry{ID string, Msg Message}` wraps `Message` with the Vault path key (required for deletion, since `Message` has no ID field of its own).

`SharePayload.WrappedKey []byte` and `SharedLink.WrappedKey []byte` both contain JSON-marshaled `WrappedKey` structs (all three ECDH fields).

### core

`App{Vault, Identity, Service}` is the fully initialised application state. `NewApp` wires `sharing.NewService`.

`InitIdentity` (in `core.go`) handles both paths: first run (generate keypair → lock → store → publish pubkey) and returning user (retrieve locked identity → decrypt → re-publish pubkey). The re-publish on unlock keeps the directory entry's display name current and self-heals entries published before names existed. The unlock password is intentionally separate from the Vault auth credential.

`ChangePassword` (in `core.go`) re-wraps the stored locked identity under a new Argon2id-derived key: load the locked identity, unlock with the old password (verifies it; generic error on mismatch), re-lock with the new password (fresh salt), and write back in a single replacement `PutLockedIdentity`. The keypair is unchanged, so no item contents are re-encrypted and the live session stays valid; a write failure leaves the old record intact and unlockable. UI is `ui/password.go`'s change-password dialog, reached from the main window's hamburger menu (`showMainMenu`, popup anchored to the menu button).

`RotateKey` (in `core.go`) does full key rotation for compromise recovery: generate a new keypair, re-encrypt every owned item under a fresh item key wrapped to the new key, re-distribute outstanding shares to recipients' current public keys, publish the new public key, and destroy the old keypair. It is staged for crash-safety: the old key is written to `identity.prev` *before* the new key becomes canonical, and `identity.prev` is deleted only once every owned item is migrated (the point the old key is destroyed). Ordering invariant: the old key must remain recoverable until all owned items are migrated. `completeInterruptedRotation` (shared by `RotateKey` and `InitIdentity`'s unlock path) finishes an interrupted rotation — it recognises the aborted-before-commit case (canonical and prev share a fingerprint) and cleans up, otherwise runs `Service.Rekey` with the old key to read and the new key as target. `InitIdentity` calls it on every unlock, so an interrupted rotation completes before the main window opens. The bulk re-key itself is `sharing.Service.Rekey` (idempotent/resumable: an item already openable with the new key is not re-encrypted; shares are reconciled to the item's current key each pass). UI is `ui/password.go`'s rotate-key dialog (warning + password), reached from the same hamburger menu. The in-memory identity/service swap goes through `App.adoptIdentity`. Items shared *with* the user (wrapped to their old key by other owners) are not re-keyed by rotation — only those owners can; they degrade to unreadable rows until re-shared.

### ui

`NewSetupWindow` handles the first-run flow: collects vault address, mount path, auth method, and credentials; validates; authenticates; verifies mount access; saves config; opens the main window.

`NewUnlockWindow` (in `unlock.go`) checks whether a locked identity exists in Vault asynchronously, then swaps the window body to show either a "set password" form (first run, with confirmation) or a "enter password" form (returning user). On submit, calls `core.InitIdentity` in a goroutine, then `onUnlock(app)` on the main thread.

`NewMainWindow` (in `app.go`) is the item list/editor: HSplit master-detail with toolbar (new/refresh), search + type filter, and a status bar with retry. `fields.go` holds the descriptor table (`typeSpecs`) mapping each item type to its fields; both the read-only detail view (`detail.go`, mask/reveal/copy) and the editors (`editor.go`, including the custom-fields repeater) are generated from it. `model.go`'s `loadRows` processes the inbox, decrypts everything readable, fetches the directory name map, turns decrypt failures into "unreadable" rows, and silently deletes dead SharedLinks (envelope 404 = missed revoke). Shared items are read-only in the UI.

`share.go` holds the sharing UI: an async "Sharing" section in owned items' detail views (access list from ShareRecords, per-recipient revoke with confirmation) and the share dialog (eligible recipients = directory minus self minus current recipients; duplicate/empty names disambiguated with an entity-ID prefix). Entity IDs never appear raw except as that fallback prefix.

## Conventions

- Config structs live in `internal/config`. Sub-configs are passed to components (e.g. `NewVault(cfg config.Vault)`) rather than spread across packages.
- Defaults are defined once via `creasty/defaults` struct tags, never duplicated in `viper.SetDefault()`.
- Go prohibits defining methods on types from other packages; structure accordingly.

## Vault setup

- Vault v2.0.0 at `vault.avitac.co:8200`
- KV v2, mount: `cowbird`
- userpass auth, mount accessor: `auth_userpass_1c802641`
- Policy: `cowbird-user-access`, using `{{identity.entity.id}}` templating — checked into the repo as `cowbird-user-access.hcl` (keep the live policy and the file in sync)
- Vault ACL rules do NOT merge: the most specific matching path wins outright, so an exact templated path shadowed by a glob must repeat the glob's capabilities (this bit us: own-pubkey rule needs explicit `read` despite `read` on `data/pubkeys/*`)
- Vault userpass emits no group claims in v2.0.0, so external group auto-enrollment does not work. Policy must be set directly on users. Per-user `token_policies` is the current workaround and is not ideal; revisit policy assignment at scale.

## On the horizon

- Key export/import UI (`crypto.ExportKey`/`ImportKey` are implemented; UI is not)
- Vault policy update: confirm `users/<eid>/identity`, `users/<eid>/links/*`, and `users/<eid>/shares/*` paths (one self-subtree rule covers all); confirm inbox hard-delete on metadata path
- Vault policy assignment at scale
- Mobile implementation (currently stubbed)
- CLI interface
- TOFU change detection (deliberately deferred; see `specs/001-sharing-and-items/research.md`)
- Optional Ed25519 authorship signing (deferred; slot reserved in `sharing.Envelope.Signature`)

## Gotchas

- Fyne widget values must be captured on the main thread before launching goroutines. All Fyne widget updates from goroutines must use `fyne.Do()` — and that includes *window creation and `Show()`*, not just widget mutation. Watch completion callbacks: a callback invoked from a worker goroutine (e.g. setup's `onComplete`) silently carries the wrong thread into any UI work it does; Fyne logs "Error in Fyne call thread" warnings when this happens.
- `widget.NewFormItem` is not a `fyne.CanvasObject`, so it cannot be added to `container.NewVBox`.
- `viper.ConfigFileNotFoundError` is not triggered when using `SetConfigFile`; check with `os.IsNotExist` instead.
- `time.Duration` serializes as raw nanoseconds through mapstructure/viper.
- vault-client-go's KV response parser does NOT use `json.Number`. When reading raw `map[string]interface{}` from response metadata (e.g. to extract the `version` field), numbers arrive as `json.Number` only if you set `decoder.UseNumber()` yourself — otherwise they silently become `float64`, which loses precision for large version integers. `vault/store.go`'s `versionFromMetadata` type-asserts to `json.Number` and calls `.Int64()`.
- `KvV2DeleteMetadataAndAllVersions` returns a nil response body on success (204 No Content). Accessing fields on a nil response panics; guard with a nil check or just ignore the response.
- KV v2 list returns a 404 when no keys exist yet under a prefix; `vault/store.go`'s `kvList` treats a 404 as an empty list, not an error.
- `Message` has no `ID` field — the Vault path component that identifies the inbox message is external to the struct. `sharing.InboxEntry{ID, Msg}` exists specifically to carry that path key out of `ListInboxMessages` so callers can pass it to `DeleteInboxMessage`.

## Sharing and items design

See the spec set for the current design:
@specs/001-sharing-and-items/spec.md
@specs/001-sharing-and-items/plan.md
@specs/001-sharing-and-items/data-model.md
@specs/001-sharing-and-items/research.md