# Implementation Plan: Sharing Hardening

**Branch**: `008-sharing-hardening` | **Spec**: [spec.md](./spec.md)

## Technical Context

Same stack as 001 (Go 1.26, Vault KV v2, `golang.org/x/crypto`). No new
dependencies expected: Ed25519 is in the stdlib (`crypto/ed25519`), already
imported and reserved in `crypto.Identity`. The work is spread across
`crypto`, `sharing`, `vault`, `core`, and `ui`.

Each numbered item below is independently shippable. Recommended order is
risk-reduction-per-effort, not strict dependency — only signing (item 4) has a
prerequisite (the signing-key migration, item 4a).

## Item 1 — Path-authority for share owner (FR-001) [quick win]

**Status: implemented 2026-06-15.** `processShare` now derives the owner from the
`SharePath` prefix, discards (consumes without linking) any message whose claimed
`OwnerID` disagrees or whose path is malformed, and stores the path-derived owner
in the `SharedLink`. Covered by `TestProcessShareRejectsForgedOwner`.

The single cheapest, highest-value change; no schema change, no migration.

- The `shared/<ownerEntityID>/<shareID>` path is the one owner attribution Vault
  actually authenticates: policy `shared/{{identity.entity.id}}/*` means only
  entity X can write under `shared/X/`. So the path prefix is trustworthy; the
  `SharePayload.OwnerID` field is not.
- In `sharing.Service.processShare`: parse the owner from `Share.SharePath`
  (already done at open time by `parseSharePath`) and (a) reject the message if
  `Share.OwnerID != pathOwner`, (b) populate `SharedLink.OwnerID` from the path
  owner regardless. `ui/model.go` keeps reading `link.OwnerID`, now trustworthy.
- Net effect: a forged message naming someone else as owner but pointing at the
  forger's own namespace is dropped. Does not yet stop a forger who correctly
  names *their own* namespace (that is honest attribution) — item 4 covers
  authenticity of the binding itself.

## Item 2 — Authenticated associated data (FR-006)

**Status: implemented 2026-06-15.** `crypto.Seal`/`Open` take an `aad` parameter
(callers that don't need it pass `nil`). Envelope content binds `contentAAD(OwnerID,
Type)`; an `Envelope.Format` field (0 = legacy nil-AAD, 1 = bound) drives a
back-compatible open path, and any write (`CreateItem`/`UpdateItem`/re-key)
upgrades to Format 1 — lazy migration, no blocking pass. **Deviation from the
original plan:** `ID` is not bound (shared copies reuse the ciphertext under a
different ID); see FR-006. Tests: `TestSealOpenWithAAD`,
`TestEnvelopeMetadataIsAuthenticated`, `TestLegacyEnvelopeStillOpens`.

- `crypto.Seal`/`Open` gain an `aad []byte` parameter (or sibling
  `SealWithAAD`/`OpenWithAAD`; XChaCha20-Poly1305 already accepts additional
  data — currently passed `nil`).
- Envelope content seal/open binds a canonical encoding of the stable metadata:
  `OwnerID`, `ID`, `Type`. Item key wrapping can likewise bind the recipient ID.
- **Migration (FR-008)**: legacy envelopes were sealed with `nil` AAD. Open MUST
  try the bound AAD and fall back to `nil` for legacy items, OR a one-time
  re-seal of owned items on unlock. Decision: lazy re-seal — on any `UpdateItem`/
  rotation the envelope is rewritten with AAD; `Open` accepts both forms until
  then. Avoids a blocking migration pass. Record the chosen form with a version
  marker on the envelope if needed to avoid trial-decrypt ambiguity.

## Item 3 — Re-key on revoke and on delete (FR-004, FR-005)

**Status: implemented 2026-06-15.** `Revoke` now re-keys the item under a fresh
item key and redistributes to the remaining recipients (the revoked share
excluded) *before* dropping the revoked copy — keyed off the still-present share
record so a partial failure is retryable. A shared `resealUnderNewItemKey` helper
backs both rotation and revoke re-keying. `DeleteItem` uses the non-re-keying
`dropShare` (the item is about to be destroyed). The revoked copy's version
history is destroyed by the existing hard-delete; co-recipient history holds only
already-seen content, so it is left as-is (noted below). Test:
`TestRevokeRekeysSoRetainedKeyIsDead` (multi-recipient: revoked key cannot open
the re-keyed co-recipient copy).

Make revocation cryptographic, reusing the rotation machinery
(`Service.rekeyOwnedItem` + `redistributeShares`).

- `Revoke(shareID, recipientID)` becomes: load owned item → generate fresh item
  key → re-encrypt owned envelope → re-wrap to **remaining** recipients (all
  share records for the item except the revoked one) → rewrite their shared
  envelopes (new version) and notify them with the new wrapped key → hard-delete
  the revoked recipient's envelope copy (existing `DeleteMetadataAndAllVersions`,
  which already destroys its version history, satisfying FR-005 for that path) →
  send revoke message → delete the revoked share record.
- Ordering/idempotency: keep the existing "remaining recipients first, then drop
  the revoked copy" discipline so a partial failure is retryable. The revoked
  copy must not be the last thing standing between a co-recipient and the new
  key.
- Note on co-recipient version history: their old-key versions remain in KV
  history but contain only content the revoked user already had access to, so
  they are not a new exposure. Destroying them is optional defence-in-depth, not
  required for FR-004.
- `DeleteItem` already revokes every share first; it inherits re-keying for free
  once `revokeShare` re-keys (the final `DeleteItem` then hard-deletes the owned
  envelope).

## Item 4 — Signed shares (FR-002, FR-003) [largest]

**Status: implemented 2026-06-15.** `NewIdentity` now also generates an Ed25519
keypair; `Identity.EnsureSigningKey` is the legacy-migration path, invoked on
unlock (`InitIdentity`) and import (`ImportIdentity`), which re-locks/persists and
re-publishes. `pubkeyRecord`/`PublicKeyEntry` carry `SigPub`; `Store` gained
`GetSigningKey` and `PutPublicKey` takes the signing key (no new Vault path, no
policy change — it rides in the existing `pubkeys/<eid>` record). Messages carry a
`Signature` over a canonical, length-prefixed, domain-separated byte string
(`internal/sharing/signing.go`); senders sign in Share/redistribute/revoke,
recipients verify in `processShare` (against the path owner) and `processRevoke`
(against the link owner). Verification is lenient only when the signer has
published no key yet (migration window); a published key makes a valid signature
mandatory and rejects unsigned/forged/downgraded messages. Tests:
`TestProcessShareRejectsBadSignature`, `TestProcessRevokeRejectsForgedRevoke`,
plus crypto `TestLockUnlockPreservesSigningKey`/`TestEnsureSigningKey`.

### 4a. Publish signing keys (prerequisite, also a migration)

- Extend `vault/store.go` `pubkeyRecord` with a `SigPub []byte` field (records
  without it unmarshal with an empty signing key, like the earlier `Name`
  addition in 003).
- Populate `crypto.Identity.Signing{Pub,Priv}` (currently reserved/deferred):
  `NewIdentity` generates the Ed25519 pair; `lockedKeys.SigningPriv` already has
  a slot, so `LockIdentity`/`UnlockIdentity` need only start writing/reading it.
- Migration: on unlock, if the locked identity has no signing key, generate one,
  re-lock, and re-publish the directory entry with `SigPub`. Mirrors the existing
  pubkey re-publish in `core.InitIdentity`.

### 4b. Sign and verify messages

- Define the signed byte string canonically: e.g. `Type || ShareID || OwnerID ||
  EnvVersion || SharePath || SHA-256(WrappedKey)` for share, `Type || ShareID`
  for revoke. Avoid signing the whole JSON (non-canonical); sign a fixed-order
  concatenation or a sub-struct with deterministic encoding.
- `Message` gains a `Signature []byte`. Sender signs with `SigningPriv` in
  `Service.Share`/`Revoke`/`redistributeShares`; recipient verifies in
  `processShare`/`processRevoke` against `store.GetPublicKey`'s signing part for
  `SenderID`.
- Failure handling: a message that does not verify is discarded (and the inbox
  entry deleted, or quarantined) and never becomes a readable row. Surface a
  count of discarded/untrusted messages rather than silently dropping.
- Backstop: combine with item 1 — even a validly signed message must still pass
  the path-owner check, and `SenderID` for a share should equal the path owner.

## Item 5 — Inbox robustness (FR-007) [independent]

**Status: implemented 2026-06-15.** `processEntry`/`processShare` discard
malformed, oversized, forged, and unknown-type messages (consume without acting)
instead of returning an error that would abort the whole inbox — closing the
one-message startup-brick. `ProcessInbox` caps work at `maxInboxPerRun` (256) so a
flood degrades to a slower drain; `shareWithinLimits` bounds the attacker-
controlled fields. Test: `TestProcessInboxDiscardsBadMessages`. **Not done:**
moving inbox processing fully off the UI startup path is a Fyne-threading change
left for the UI layer — `loadRows` already runs in a worker goroutine, so the
window is not frozen today; the per-run cap bounds the list delay. Tracked as a
follow-up.

- Reject oversized inbox messages on read (cap serialized size).
- Bound `ProcessInbox`: cap messages processed per run and/or move inbox
  processing off the UI-blocking startup path (`ui/model.go`'s `loadRows` calls
  it first thing). The window should open before the inbox fully drains.
- These are client-side; Vault policy cannot easily enforce per-user quotas
  (noted in spec Out of Scope).

## Test Strategy

- `crypto`: AAD round-trip, legacy no-AAD decrypt acceptance, Ed25519
  sign/verify, locked-identity signing-key round-trip and migration.
- `sharing` (in-memory `Store`): forged-owner rejection (item 1); re-key-on-
  revoke leaves the revoked key unable to open post-revoke content while
  remaining recipients reconcile (item 3); signed-message accept/reject paths
  (item 4); oversized/over-count inbox handling (item 5).
- `core`: signing-key migration on unlock for a legacy identity.

## Open Items

- AAD migration marker: trial-accept-both vs explicit per-envelope version byte —
  pick during item 2 to avoid ambiguous trial-decrypt.
- Whether to quarantine vs hard-delete unverified inbox messages (audit value vs
  inbox bloat).
- Canonical signing encoding: hand-rolled fixed-order concat vs a small
  deterministic codec; decide in item 4b.
- Confirm Vault honours hard-delete of the revoked path's full version history
  under the owner-templated `shared/<eid>/*` delete capability (should, via
  `metadata/shared/<eid>/*` `delete`).
