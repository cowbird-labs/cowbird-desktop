# cowbird

A desktop password manager written in Go that uses [HashiCorp Vault](https://www.vaultproject.io/) as its storage backend. Cowbird has no server component of its own — just you and your Vault.

- **GUI**: [Fyne](https://fyne.io/) (`fyne.io/fyne/v2`), app ID `co.avitac.cowbird`
- **Targets**: macOS / Linux / Windows desktop now; Android / iOS eventually
- **Go**: 1.26

## Design intent

Cowbird is built for deployments with a **known, finite set of users** — a company team or a family — where each deployment runs its own Vault. Within that scope it aims to provide:

1. **End-to-end encrypted storage.** All item contents are encrypted on the client. The Vault operator can never read them.
2. **Item sharing between users.** A user can share an individual item with another user, who can then read it (and see subsequent edits) without the content ever being re-encrypted per recipient.
3. **No operator-side recovery.** The only recovery mechanism is a user-initiated, passphrase-protected export of the private key. There is deliberately no admin reset: no trusted party exists who could perform one without also being able to read your data.

## Status

- [x] **Crypto layer** — Argon2id/HKDF KDF, XChaCha20-Poly1305 seal/open, X25519 key wrapping, identity lock/unlock, key export/import (with tests)
- [x] **Item types and codec** — all six content types, custom fields, type-dispatch encode/decode (with tests)
- [x] **Vault integration** — auth (userpass/token/AppRole), background token renewal, mount verification, full `sharing.Store` implementation, locked-identity storage
- [x] **Sharing service** — create/open items, share, revoke, idempotent inbox processing (with integration tests against an in-memory store)
- [x] **First-run setup UI** — Vault address/mount/auth collection, validation, config save
- [x] **Unlock UI** — set-password (first run) and enter-password (returning user) flows; identity creation and unlock
- [x] **Config & credential storage** — TOML config, OS-keyring credential store
- [x] **Item list / editor UI** — searchable master-detail window: create/edit/delete for all six types with custom fields, masked sensitive values with reveal/copy, shared items read-only
- [x] **Share / revoke UI** — share an owned item with a user picked by display name, see who has access, revoke per recipient; display names ride the auth identity and are published with public keys
- [x] **Password change flow** — `core.ChangePassword` re-wraps the locked identity under a new Argon2id key (no item re-encryption); reached from the main window's hamburger menu
- [ ] **Key rotation flow** — same: primitives done, no service/UI
- [ ] **Key export/import UI** — `crypto.ExportKey`/`ImportKey` are implemented; no UI
- [x] **Vault policy** — reference policy checked in as [`cowbird-user-access.hcl`](cowbird-user-access.hcl), verified against the live deployment (including the pubkey-directory list and the ACL precedence fix for the own-pubkey rule)
- [ ] **Policy assignment at scale** — userpass in Vault 2.0.0 emits no group claims, so the policy is set per user via `token_policies`; revisit before deployments grow
- [ ] **CLI interface**
- [ ] **Mobile** — credential store is stubbed; no mobile builds
- [ ] **TOFU change detection** — deliberately deferred (see trust model)
- [ ] **Ed25519 authorship signing** — deferred; envelope field reserved


### Trust model: semi-trusted operator

The storage operator is treated as **semi-trusted**: technically prevented from reading item contents, but assumed not to actively forge user identities (Decision 3 in [research.md](specs/001-sharing-and-items/research.md)). In both target deployments the operator is accountable and non-adversarial — a corporate Vault team, or the user themselves.

Consequences of this choice:

- Public keys are published to a directory in Vault and trusted as-is. Mandatory out-of-band fingerprint verification is **not** required (the friction causes users to skip it anyway, yielding the appearance of zero-trust without the substance).
- A malicious operator could substitute a public key on first contact. This is explicitly accepted, not solved. TOFU change detection is deferred, and the machinery would allow an optional "verify this contact" affordance later without redesign.
- Vault path ACLs are **not** the security boundary for shared items — the wrapped item key is. Broad read access on the shared namespace is safe only because contents are encrypted.

### Separation of credentials

Three distinct secrets exist by design:

| Secret | Purpose | Where it lives   |
|---|---|------------------|
| Vault auth credential (userpass / token / AppRole) | Authenticates to Vault | OS keyring       |
| Unlock password | Decrypts the user's private key | User's head only |
| Export passphrase | Protects an exported key file | User's head only |

The unlock password is intentionally separate from the Vault credential, so an operator-side credential reset cannot decrypt data. Following the Bitwarden model, **changing the unlock password** only re-wraps the key material (cheap), while **rotating the key** re-secures all items (expensive, for compromise scenarios) — these are distinct operations.

## Encryption

### Envelope model

Each item has its own randomly generated 32-byte symmetric **item key**:

- Item content is encrypted once with the item key using **XChaCha20-Poly1305**.
- The item key is **wrapped** (encrypted) to each authorized recipient's **X25519** public key via ephemeral ECDH + HKDF + XChaCha20-Poly1305. Adding a recipient means adding one wrapped key; content is never re-encrypted.

### Primitives

| Purpose | Primitive | Notes |
|---|---|---|
| Content & key encryption | XChaCha20-Poly1305 | `Seal`/`Open` in `internal/crypto/aes.go`; `Open` returns a generic `"decryption failed"` error to avoid leaking failure mode |
| Key wrapping | X25519 (via Go stdlib `crypto/ecdh`) | Ephemeral ECDH + HKDF per wrap; `EphemeralPub`, `Nonce`, and `Wrapped` stored explicitly |
| Password-derived keys | Argon2id (time=3, memory=64 MB, threads=4, keyLen=32) | With HKDF domain separation: `"cowbird-unlock-v1"` for unlocking the identity, `"cowbird-wrap-v1"` internal to key wrapping |
| Key fingerprint | SHA-256 of the X25519 public key, hex-encoded | |
| Authorship signing | Ed25519 — **deferred** | Slot reserved in `Envelope.Signature`; only matters under an adversarial-operator model |

### Key lifecycle

- A user's `Identity` (X25519 keypair) is generated client-side on first run.
- At rest it is stored in Vault as a `LockedIdentity` — the private key encrypted under an Argon2id-derived key. Private keys exist in plaintext only in client memory after unlock.
- The public key is published to the Vault pubkey directory for others to wrap item keys to.
- `ExportKey`/`ImportKey` produce/consume a passphrase-protected JSON blob (`ExportedKey{Version, Salt, Nonce, Ciphertext}`, currently version 1) for device-loss recovery.

## Sharing protocol

Shared envelopes live in a shared namespace (single copy — edits propagate without re-sharing); per-recipient wrapped keys travel privately via a **consume-and-delete inbox**, never in the shared blob, so "who has access" stays out of the shared-readable namespace.

- **Share**: the owner writes/updates the shared envelope and drops a `share` message (containing the recipient's wrapped item key) into the recipient's inbox. The recipient's client consumes it, writes a durable **SharedLink** record into its own subtree, then deletes the message.
- **Revoke**: the owner removes/re-keys the shared envelope (the real security action) and drops a `revoke` message; the recipient's client removes its link. A missed revoke message degrades to a dead link, not retained access.
- **Robustness**: inbox processing is idempotent — write/remove the link *first*, then delete the message, so partial failures self-heal on reprocess. Out-of-order and duplicate delivery are resolved using the shared envelope's server-assigned KV v2 version number (not a client counter or timestamp).

### Vault storage layout (KV v2, mount `cowbird`)

```
users/<entityID>/items/<itemID>    # owner's own items
users/<entityID>/identity          # locked (encrypted) keypair
users/<entityID>/links/<shareID>   # durable SharedLink records
users/<entityID>/pinned            # encrypted pinned-keys record (reserved, not yet used)
pubkeys/<entityID>                 # public keys + display names (read-all, write-own)
shared/<ownerEntityID>/<shareID>   # shared item envelopes
inbox/<recipientEntityID>/<msgID>  # transient share/revoke messages
```

All key names are opaque UUIDs — Vault list output is not ACL-filtered, so nothing meaningful may appear in key names. Every KV write stores a `{"v": "<json-string>"}` wrapper around the JSON-marshaled struct, keeping the read/write path uniform.

The per-entity Vault policy (using `{{identity.entity.id}}` templating) gives users full CRUD on their own subtree, read-all on `pubkeys/*` and `shared/*` with write only on their own entries, and **create-only** access to other users' inboxes — a sender can drop a new message but cannot read, list, or overwrite. These KV v2 `create`-vs-`update` semantics were verified empirically against a running Vault.

## Data structures

### Items (`internal/items`)

One `Content` interface with concrete typed structs — `Login`, `Card`, `Note`, `Identity`, `Password`, and freeform `Custom` — each carrying a `CustomFields []Field` slice for arbitrary extra fields (`Field{Type, Label, Value}` with field types `text`, `hidden`, `totp`, `url`). This keeps compile-time guarantees and easy autofill on known fields while allowing Proton-style flexibility.

Since JSON can't unmarshal into an interface, `Encode`/`Decode` (`codec.go`) use a `{"type": "...", "data": {...}}` envelope for type dispatch.

### Sharing (`internal/sharing`)

```go
type WrappedKey struct {           // item key encrypted to one recipient
    RecipientID  string
    EphemeralPub []byte            // X25519 ephemeral public key
    Nonce        []byte
    Wrapped      []byte
}

type Envelope struct {             // an encrypted item at rest
    ID, OwnerID string
    Type        ItemType
    Recipients  []WrappedKey       // owner's own access; shared recipients get keys via inbox
    Nonce       []byte
    Ciphertext  []byte             // content encrypted with the item key
    Signature   []byte             // Ed25519, reserved/deferred
}

type Message struct {              // transient inbox message (share or revoke)
    Type       MessageType        // "share" | "revoke"
    ShareID    string
    SenderID   string
    EnvVersion int64               // KV v2 version of the envelope; ordering tiebreaker
    Timestamp  time.Time           // display only
    Share      *SharePayload       // share messages only: SharePath, WrappedKey, ItemType, OwnerID
}

type SharedLink struct {           // recipient's durable record of a share
    ShareID, SharePath, OwnerID, ItemType string
    WrappedKey []byte              // JSON-marshaled WrappedKey
    EnvVersion int64               // last-seen version acted on
}
```

`InboxEntry{ID, Msg}` pairs a `Message` with the Vault path key needed to delete it (the message struct itself has no ID). `SharePayload.WrappedKey` and `SharedLink.WrappedKey` are JSON-marshaled `WrappedKey` structs.

### Key material (`internal/crypto`)

```go
type Identity struct {             // in-memory only, after unlock
    SigningPub     ed25519.PublicKey  // deferred
    SigningPriv    ed25519.PrivateKey
    EncryptionPub  [32]byte           // X25519
    EncryptionPriv [32]byte
    Fingerprint    string             // hex SHA-256 of EncryptionPub
}

type LockedIdentity struct { Salt, Nonce, Ciphertext []byte }          // at rest in Vault
type ExportedKey   struct { Version int; Salt, Nonce, Ciphertext []byte } // recovery file
```

Full definitions and rationale: [data-model.md](specs/001-sharing-and-items/data-model.md).

## Project structure

```
main.go               # entry point; hand-coded dependency injection (no Wire)
internal/
├── auth/             # Vault auth: Method interface; Userpass, Token, AppRole
├── config/           # Viper + TOML config (~/.config/cowbird/config.toml), struct-tag defaults
├── credentials/      # CredentialStore on the OS keyring; mobile stub via build tags
├── vault/            # Vault client wrapper, token renewal; implements sharing.Store
├── crypto/           # KDF, XChaCha20-Poly1305, X25519 wrapping, identity, export/import
├── items/            # item content types + JSON codec
├── sharing/          # envelope crypto, inbox protocol, share/revoke service
├── core/             # App state, identity init (first-run and returning-user)
└── ui/               # Fyne UI: setup, unlock, main window
```

Core logic is decoupled from the UI so a CLI interface can be added later.

## Development

```sh
go build ./...
go test ./...
go run .
```

Requires a reachable Vault (KV v2 engine) with a per-entity templated policy as described above. Design documents live in [`specs/001-sharing-and-items/`](specs/001-sharing-and-items/): [spec](specs/001-sharing-and-items/spec.md), [plan](specs/001-sharing-and-items/plan.md), [data model](specs/001-sharing-and-items/data-model.md), and [decision record](specs/001-sharing-and-items/research.md).

## License

Copyright (C) 2026 Anthony Vitacco

Cowbird is free software, licensed under the [GNU General Public License v3.0](LICENSE). You may redistribute and modify it under the terms of that license; derivative works must remain open source under the same terms.
