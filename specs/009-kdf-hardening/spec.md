# Feature Specification: KDF Hardening

**Feature Branch**: `008-model-hardening` (implemented alongside 008)
**Status**: Implemented 2026-06-15
**Created**: 2026-06-15

## Summary

The unlock password is the single gate between a typed secret and every private
key in cowbird, so the password-hashing KDF (Argon2id) is the most security-
sensitive tuning decision in the app. This feature (a) removes a dead, confusing
`[argon]` config section, (b) raises the Argon2id work factor to a robust fixed
value, and (c) makes future increases non-breaking via a per-record KDF version
marker. Parameters are intentionally **not** user-tunable.

## Background — why this came up

The config file carried an `[argon]` section (`time`, `memory`, `threads`,
`key_len`) that *looked* security-critical but was **never read** — the KDF used
hardcoded constants in `internal/crypto/kdf.go`. Worse, the on-disk values were
all `0` (Save wrote a zero-valued struct), so the section read as if password
hashing were disabled. It was a footgun: someone could "harden" those numbers,
or panic at the zeros, while changing nothing. Investigating it led to the
decision to either delete it or wire it up — and, having looked at the cost of
wiring it up, to delete it and instead set genuinely strong fixed parameters.

## Decisions (decision record)

### D1 — Fixed parameters, not user-tunable

**Decision**: Argon2id parameters are fixed constants in code, not configurable.

**Rationale**: Making them tunable *correctly* requires storing the parameters
used to lock **each** record (like a PHC hash string does), because changing the
parameters changes the derived key and would otherwise make every existing
`LockedIdentity` and recovery file undecryptable. That per-record-params
machinery is real complexity for a knob almost no one should touch. The better
answer is to pick parameters robust enough that tuning is unnecessary.

**Alternative rejected**: a `[argon]` config with safe floors/defaults and a
below-floor warning. Sound, but it still needs per-record param storage to be
correct, and invites misconfiguration. Not worth it for the target users.

### D2 — Raise time 3 → 25, keep memory at 64 MiB

**Decision**: `kdfV2 = {time: 25, memory: 64 MiB, threads: 4, keyLen: 32}`.

**Rationale**: cowbird targets mobile, so memory is the constrained axis — RFC
9106's *first* recommended profile (2 GiB, t=1) would OOM phones and stall even
desktops, so it is out. The *second* recommended profile (64 MiB, t=3) is the
mobile-feasible base; we keep its 64 MiB and buy additional strength with
iterations. Measured cost on the developer's machine (AMD Ryzen 5 2600X, the
benchmark since removed):

| t | per unlock | vs t=3 | added work factor |
|---|---|---|---|
| 3 (old) | ~57 ms | 1x | — |
| 10 | ~187 ms | 3.3x | +1.7 bits |
| **25 (new)** | **~401 ms** | **7x** | **+3.1 bits** |
| 50 | ~803 ms | 14x | +4.1 bits |

Cost is linear in `t` (~16–19 ms/iteration). t=25 lands at ~0.4 s on this
(2018-era) desktop and an estimated ~1–2 s on a phone (~2–5x slower) — the
strongest value that keeps the worst-case mobile unlock in the low seconds.
Higher `t` (e.g. 10000 → minutes per unlock) is self-defeating: the cost is
asymmetric in the user's favour already (the attacker pays it on every one of
billions of guesses; the user pays it once per session), so a multi-minute
unlock buys only a handful of bits for an unusable app.

### D3 — Per-record KDF version marker for non-breaking upgrades

**Decision**: each locked record stores the KDF *version* it was derived under;
unlock derives with that version's parameters; new locks use the current
version. Absent/`0` means a pre-009 record and maps to `kdfV1`.

**Rationale**: even a *fixed* parameter change breaks existing records unless the
record remembers how it was derived. A one-int version marker (not the full
parameter set, since we are not exposing tuning) makes raising the work factor a
non-breaking change forever — the same lazy, versioned-migration pattern already
used for the AAD `Format` field (008 Item 2).

## Requirements

- **FR-001**: The dead `[argon]` config section and its struct MUST be removed.
- **FR-002**: Argon2id parameters MUST be fixed in code (not user-configurable)
  and MUST be at least kdfV2 (t=25, m=64 MiB, p=4, keyLen=32) for new records.
- **FR-003**: Each at-rest password-locked record (`LockedIdentity`, exported
  recovery file) MUST record the KDF version used to derive its key.
- **FR-004**: Unlock MUST derive using the parameters of the record's stored
  version, so identities and recovery files created under older parameters
  continue to open without any reset.
- **FR-005**: On unlock, a record below the current KDF version MUST be
  transparently re-locked under the current version (password already in hand),
  so existing identities strengthen on next use with no user action.
- **FR-006**: Raising the KDF strength in future MUST be possible by adding a new
  version to the table without breaking existing records.

## Out of Scope

- User-facing tuning of Argon2id parameters (D1).
- Per-deployment parameter policy.
- Rewriting the user's existing config file to physically delete the now-inert
  `[argon]` block (Save merges rather than rewrites; the block is ignored on
  load and can be removed by hand).

## Assumptions

- Target users run cowbird on a mix of desktop and (eventually) mobile, so the
  parameter floor must remain mobile-feasible.
- The unlock password is the only secret protecting the private keys; the Vault
  credential is separate (001 trust model), so KDF strength is the relevant
  brute-force defense.
