# Implementation Plan: KDF Hardening

**Branch**: `008-model-hardening` | **Spec**: [spec.md](./spec.md)
**Status**: Implemented 2026-06-15

Implemented directly (decisions were settled in discussion); this plan documents
what was built. Same stack as the rest of the project.

## Design

### Versioned parameter table (`internal/crypto/kdf.go`)

```go
type kdfParams struct { time, memory uint32; threads uint8; keyLen uint32 }

const ( kdfV1 = 1; kdfV2 = 2; currentKDFVersion = kdfV2 )

var kdfVersions = map[int]kdfParams{
	kdfV1: {3,  64*1024, 4, 32},  // original
	kdfV2: {25, 64*1024, 4, 32},  // 009: raised time
}
```

- `kdfParamsForVersion(v)` maps version 0 (absent) → kdfV1, errors on unknown.
- `DeriveUnlockKey(password, salt)` (public, unchanged signature) derives with
  `currentKDFVersion`; internal `deriveUnlockKey(password, salt, p)` does the
  version-specific Argon2id + HKDF.

### Per-record version (`identity.go`, `export.go`)

- `LockedIdentity` gains `Version int` (omitempty; 0 = legacy kdfV1).
- `ExportedKey` gains `KDFVersion int` (distinct from its existing `Version`,
  which is the *file-format* version). Old recovery files lack it → kdfV1.
- `LockIdentity` stamps `currentKDFVersion`; `UnlockIdentity` derives with
  `kdfParamsForVersion(locked.Version)`. `ExportKey`/`ImportKey` carry the KDF
  version through.
- `NeedsKDFUpgrade(locked)` reports whether a record is below the current version.

### Lazy upgrade-on-unlock (`core.InitIdentity`)

Folded into the single re-lock already added for the 008 signing-key migration:

```go
addedSigningKey, _ := id.EnsureSigningKey()
if addedSigningKey || crypto.NeedsKDFUpgrade(locked) {
	relocked, _ := crypto.LockIdentity(id, password) // current KDF version + signing key
	v.PutLockedIdentity(ctx, relocked)
}
```

So an existing identity is re-encrypted under t=25 on its next unlock — one Vault
write, password already in hand, no user action. All other re-lock paths
(createIdentity, ChangePassword, RotateKey, ImportIdentity) already call
`LockIdentity`, so they produce current-version records for free.

### Config (`internal/config/config.go`)

`Argon` struct and the `Config.Argon` field removed. No reader existed. The
decoder does not set `ErrorUnused`, so a lingering `[argon]` block in a user's
file is ignored on load (Save merges, so it is not auto-removed — documented as
out of scope).

## Files touched

- `internal/crypto/kdf.go` — versioned param table, split derive functions.
- `internal/crypto/identity.go` — `LockedIdentity.Version`, version-aware
  unlock, `NeedsKDFUpgrade`.
- `internal/crypto/export.go` — `ExportedKey.KDFVersion` carried through.
- `internal/core/core.go` — lazy upgrade-on-unlock (merged re-lock).
- `internal/config/config.go` — removed `Argon`.

## Tests

- `TestLegacyKDFRecordUnlocksAndUpgrades` (identity_test.go): a hand-built v1
  record (Version 0, kdfV1 params) still unlocks and round-trips, is flagged by
  `NeedsKDFUpgrade`, and a freshly locked record is `currentKDFVersion` and not
  flagged.
- Existing crypto tests now exercise t=25 (suite ~8 s); back-compat of
  export/import covered by existing export tests plus the KDFVersion plumbing.

## Notes / follow-ups

- Raising strength later = add `kdfV3` to the table and bump `currentKDFVersion`;
  existing records keep opening and upgrade lazily. No format change.
- Core layer remains untested per project precedent (needs a live Vault); the
  upgrade-on-unlock path is exercised indirectly via the crypto primitives.
- The benchmark used to pick t=25 was a throwaway (`kdf_bench_test.go`, removed);
  numbers are recorded in spec.md D2.
