# Feature Specification: Password & Passphrase Generator

**Feature Branch**: `010-password-generator`
**Status**: Draft
**Created**: 2026-06-17

## Summary

A password manager should help users create strong secrets, not just store ones
they typed themselves. Cowbird currently has a strength meter (`ui/strength.go`)
but no way to generate. This feature adds a cryptographically-secure generator
with two modes — random-character **passwords** and word-based **passphrases**
(EFF long wordlist, diceware-style) — reachable both inline next to password
fields in the item editor and as a standalone dialog from the main menu.
Generation logic lives in a UI-independent package so the planned CLI can reuse
it. The user's last-used generator settings persist in the existing TOML config.

## User Scenarios

### User Story 1 — Generate a password while creating an item (Priority: P1)

A user editing a login wants a strong password without inventing one.

**Why this priority**: This is where a generator earns its keep — at the moment
of filling a credential field. Everything else is convenience around it.

**Acceptance Scenarios**:

1. **Given** a user is editing an item with a password field, **When** they tap
   the generate affordance beside that field, **Then** a generator opens, and
   choosing "Use" replaces the field's contents with the generated value.
2. **Given** the generator is open, **When** the user adjusts length or
   character classes and regenerates, **Then** a new value reflecting those
   options appears, with a live strength/entropy readout.
3. **Given** the user cancels the generator, **When** it closes, **Then** the
   field they were editing is left unchanged.

### User Story 2 — Generate a passphrase (Priority: P1)

A user prefers a memorable multi-word passphrase over a random string.

**Acceptance Scenarios**:

1. **Given** the generator is open, **When** the user switches to passphrase
   mode, **Then** a word-based value (e.g. `correct-horse-battery-staple`) is
   produced from the EFF long wordlist.
2. **Given** passphrase mode, **When** the user changes word count, separator,
   capitalization, or the include-a-number toggle, **Then** the output and the
   entropy readout update accordingly.

### User Story 3 — Generate without saving an item (Priority: P2)

A user wants a strong value to use elsewhere (a different app, a form).

**Acceptance Scenarios**:

1. **Given** the main window, **When** the user opens the generator from the
   hamburger menu, **Then** they can generate and copy a value to the clipboard
   without creating or editing any cowbird item.

### User Story 4 — Settings persist (Priority: P3)

A user who always wants 24-character passwords or 6-word passphrases should not
re-configure every time.

**Acceptance Scenarios**:

1. **Given** a user generated with non-default options, **When** they reopen the
   generator (even after restarting the app), **Then** their last-used mode and
   options are pre-selected.

## Requirements

### Functional Requirements

- **FR-001**: All generated output MUST use a cryptographically secure random
  source (`crypto/rand`) with unbiased selection (rejection sampling, no modulo
  bias).
- **FR-002**: Password mode MUST support configurable length and independent
  toggles for lowercase, uppercase, digits, and symbols, plus an
  exclude-ambiguous-characters option.
- **FR-003**: When multiple character classes are enabled, a generated password
  MUST contain at least one character from each enabled class (subject to
  length permitting). At least one class MUST always be enabled; the UI MUST
  prevent disabling the last one.
- **FR-004**: Passphrase mode MUST draw words from the embedded EFF long
  wordlist (7776 words) and MUST support configurable word count, separator,
  per-word capitalization, and an option to insert a random digit.
- **FR-005**: The generator MUST display a strength indicator for the current
  output. For passphrases this MUST reflect true word-based entropy
  (`words × log2(wordlistSize)`), not the character-set heuristic used for typed
  passwords.
- **FR-006**: The generator MUST be reachable both (a) inline beside password
  fields in the item editor, where accepting a value fills that field, and (b)
  as a standalone dialog from the main menu, where the value can be copied
  without touching any item.
- **FR-007**: Cancelling or dismissing the generator MUST leave the originating
  field (if any) unchanged.
- **FR-008**: The user's last-used generator settings (mode and per-mode
  options) MUST persist across sessions via the existing config file.
- **FR-009**: Generation logic MUST be independent of the Fyne UI so it can be
  reused by a future CLI.

### Key Entities

- **Password options**: length, class toggles (lower/upper/digit/symbol),
  exclude-ambiguous.
- **Passphrase options**: word count, separator, capitalize, include-number.
- **Generator settings**: the persisted mode + both option sets.

## Assumptions

- The EFF long wordlist (~100 KB, public domain / CC-BY) is acceptable to embed
  in the binary via `go:embed`.
- Generated values are not stored or transmitted by the generator itself; they
  only enter Vault if the user saves an item, through the existing item path.
- The existing `passwordStrength` heuristic remains the right display for typed
  passwords; the generator adds an entropy-accurate readout for its own output.

## Out of Scope

- Pronounceable-password algorithms and custom/user-supplied wordlists.
- Password history or "previously generated" recall (generated values are
  ephemeral until saved as an item).
- Breach/pwned-password checking (needs an external service; not in scope).
- Per-site password policies / required-character rules beyond the class
  toggles.
- Auto-rotating or bulk-regenerating existing items' passwords.
